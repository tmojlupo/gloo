package syncer

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/gogo/protobuf/types"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/channelutils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	rlv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice"
	"github.com/solo-io/gloo/projects/metrics/pkg/runner"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type RunFunc func(opts bootstrap.Opts) error

func NewSetupFunc() setuputils.SetupFunc {
	return NewSetupFuncWithRunAndExtensions(RunGloo, nil)
}

// used outside of this repo
//noinspection GoUnusedExportedFunction
func NewSetupFuncWithExtensions(extensions Extensions) setuputils.SetupFunc {
	runWithExtensions := func(opts bootstrap.Opts) error {
		return RunGlooWithExtensions(opts, extensions)
	}
	return NewSetupFuncWithRunAndExtensions(runWithExtensions, &extensions)
}

// for use by UDS, FDS, other v1.SetupSyncers
func NewSetupFuncWithRun(runFunc RunFunc) setuputils.SetupFunc {
	return NewSetupFuncWithRunAndExtensions(runFunc, nil)
}

func NewSetupFuncWithRunAndExtensions(runFunc RunFunc, extensions *Extensions) setuputils.SetupFunc {
	s := &setupSyncer{
		extensions: extensions,
		makeGrpcServer: func(ctx context.Context) *grpc.Server {
			return grpc.NewServer(grpc.StreamInterceptor(
				grpc_middleware.ChainStreamServer(
					grpc_ctxtags.StreamServerInterceptor(),
					grpc_zap.StreamServerInterceptor(zap.NewNop()),
					func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						contextutils.LoggerFrom(ctx).Debugf("gRPC call: %v", info.FullMethod)
						return handler(srv, ss)
					},
				)),
			)
		},
		runFunc: runFunc,
	}
	return s.Setup
}

type grpcServer struct {
	addr   string
	cancel context.CancelFunc
}

type setupSyncer struct {
	extensions               *Extensions
	runFunc                  RunFunc
	makeGrpcServer           func(ctx context.Context) *grpc.Server
	previousXdsServer        grpcServer
	previousValidationServer grpcServer
	controlPlane             bootstrap.ControlPlane
	validationServer         bootstrap.ValidationServer
	callbacks                xdsserver.Callbacks
}

func NewControlPlane(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, callbacks xdsserver.Callbacks, start bool) bootstrap.ControlPlane {
	hasher := &xds.ProxyKeyHasher{}
	snapshotCache := cache.NewSnapshotCache(true, hasher, contextutils.LoggerFrom(ctx))
	xdsServer := server.NewServer(snapshotCache, callbacks)
	envoyv2.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsServer)
	reflection.Register(grpcServer)

	return bootstrap.ControlPlane{
		GrpcService: &bootstrap.GrpcService{
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
			BindAddr:        bindAddr,
			Ctx:             ctx,
		},
		SnapshotCache: snapshotCache,
		XDSServer:     xdsServer,
	}
}

func NewValidationServer(ctx context.Context, grpcServer *grpc.Server, bindAddr net.Addr, start bool) bootstrap.ValidationServer {
	return bootstrap.ValidationServer{
		GrpcService: &bootstrap.GrpcService{
			GrpcServer:      grpcServer,
			StartGrpcServer: start,
			BindAddr:        bindAddr,
			Ctx:             ctx,
		},
		Server: validation.NewValidationServer(),
	}
}

var (
	DefaultXdsBindAddr        = fmt.Sprintf("0.0.0.0:%v", defaults.GlooXdsPort)
	DefaultValidationBindAddr = fmt.Sprintf("0.0.0.0:%v", defaults.GlooValidationPort)
)

func getAddr(addr string) (*net.TCPAddr, error) {
	addrParts := strings.Split(addr, ":")
	if len(addrParts) != 2 {
		return nil, errors.Errorf("invalid bind addr: %v", addr)
	}
	ip := net.ParseIP(addrParts[0])

	port, err := strconv.Atoi(addrParts[1])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid bind addr: %v", addr)
	}

	return &net.TCPAddr{IP: ip, Port: port}, nil
}

func (s *setupSyncer) Setup(ctx context.Context, kubeCache kube.SharedCache, memCache memory.InMemoryResourceCache, settings *v1.Settings) error {

	xdsAddr := settings.GetGloo().GetXdsBindAddr()
	if xdsAddr == "" {
		xdsAddr = DefaultXdsBindAddr
	}
	xdsTcpAddress, err := getAddr(xdsAddr)
	if err != nil {
		return errors.Wrapf(err, "parsing xds addr")
	}

	validationAddr := settings.GetGloo().GetValidationBindAddr()
	if validationAddr == "" {
		validationAddr = DefaultValidationBindAddr
	}
	validationTcpAddress, err := getAddr(validationAddr)
	if err != nil {
		return errors.Wrapf(err, "parsing validation addr")
	}

	refreshRate := time.Minute
	if settings.GetRefreshRate() != nil {
		refreshRate, err = types.DurationFromProto(settings.GetRefreshRate())
		if err != nil {
			return err
		}
	}

	writeNamespace := settings.DiscoveryNamespace
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.WatchNamespaces, writeNamespace)

	emptyControlPlane := bootstrap.ControlPlane{}
	emptyValidationServer := bootstrap.ValidationServer{}

	if xdsAddr != s.previousXdsServer.addr {
		if s.previousXdsServer.cancel != nil {
			s.previousXdsServer.cancel()
			s.previousXdsServer.cancel = nil
		}
		s.controlPlane = emptyControlPlane
	}

	if validationAddr != s.previousValidationServer.addr {
		if s.previousValidationServer.cancel != nil {
			s.previousValidationServer.cancel()
			s.previousValidationServer.cancel = nil
		}
		s.validationServer = emptyValidationServer
	}

	// initialize the control plane context in this block either on the first loop, or if bind addr changed
	if s.controlPlane == emptyControlPlane {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		var callbacks xdsserver.Callbacks
		if s.extensions != nil {
			callbacks = s.extensions.XdsCallbacks
		}
		s.controlPlane = NewControlPlane(ctx, s.makeGrpcServer(ctx), xdsTcpAddress, callbacks, true)
		s.previousXdsServer.cancel = cancel
		s.previousXdsServer.addr = xdsAddr
	}

	// initialize the validation server context in this block either on the first loop, or if bind addr changed
	if s.validationServer == emptyValidationServer {
		// create new context as the grpc server might survive multiple iterations of this loop.
		ctx, cancel := context.WithCancel(context.Background())
		s.validationServer = NewValidationServer(ctx, s.makeGrpcServer(ctx), validationTcpAddress, true)
		s.previousValidationServer.cancel = cancel
		s.previousValidationServer.addr = validationAddr
	}

	consulClient, err := bootstrap.ConsulClientForSettings(ctx, settings)
	if err != nil {
		return err
	}

	var vaultClient *vaultapi.Client
	if vaultSettings := settings.GetVaultSecretSource(); vaultSettings != nil {
		vaultClient, err = bootstrap.VaultClientForSettings(vaultSettings)
		if err != nil {
			return err
		}
	}

	var clientset kubernetes.Interface
	opts, err := constructOpts(ctx,
		&clientset,
		kubeCache,
		consulClient,
		vaultClient,
		memCache,
		settings,
	)
	if err != nil {
		return err
	}
	opts.WriteNamespace = writeNamespace
	opts.WatchNamespaces = watchNamespaces
	opts.WatchOpts = clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: refreshRate,
	}
	opts.ControlPlane = s.controlPlane
	opts.ValidationServer = s.validationServer
	// if nil, kube plugin disabled
	opts.KubeClient = clientset
	opts.DevMode = settings.DevMode
	opts.Settings = settings

	opts.Consul.DnsServer = settings.GetConsul().GetDnsAddress()
	if len(opts.Consul.DnsServer) == 0 {
		opts.Consul.DnsServer = consulplugin.DefaultDnsAddress
	}
	if pollingInterval := settings.GetConsul().GetDnsPollingInterval(); pollingInterval != nil {
		dnsPollingInterval, err := types.DurationFromProto(pollingInterval)
		if err != nil {
			return err
		}
		opts.Consul.DnsPollingInterval = &dnsPollingInterval
	}

	// if vault service discovery specified, initialize consul watcher
	if consulServiceDiscovery := settings.GetConsul().GetServiceDiscovery(); consulServiceDiscovery != nil {
		// Set up Consul client
		consulClientWrapper, err := consul.NewConsulWatcher(consulClient, consulServiceDiscovery.GetDataCenters())
		if err != nil {
			return err
		}
		opts.Consul.ConsulWatcher = consulClientWrapper
	}

	err = s.runFunc(opts)

	s.validationServer.StartGrpcServer = opts.ValidationServer.StartGrpcServer
	s.controlPlane.StartGrpcServer = opts.ControlPlane.StartGrpcServer

	return err
}

type Extensions struct {
	// Deprecated. Use PluginExtensionsFuncs instead.
	PluginExtensions      []plugins.Plugin
	PluginExtensionsFuncs []func() plugins.Plugin
	SyncerExtensions      []TranslatorSyncerExtensionFactory
	XdsCallbacks          xdsserver.Callbacks

	// optional custom handler for envoy usage metrics that get pushed to the gloo pod
	MetricsHandler metricsservice.MetricsHandler
}

func GetPluginsWithExtensionsAndRegistry(opts bootstrap.Opts, registryPlugins func(opts bootstrap.Opts) []plugins.Plugin, extensions Extensions) func() []plugins.Plugin {
	pluginfuncs := extensions.PluginExtensionsFuncs
	for _, p := range extensions.PluginExtensions {
		p := p
		pluginfuncs = append(pluginfuncs, func() plugins.Plugin { return p })
	}
	return func() []plugins.Plugin {
		plugins := registryPlugins(opts)
		for _, pluginExtension := range pluginfuncs {
			plugins = append(plugins, pluginExtension())
		}
		return plugins
	}
}
func GetPluginsWithExtensions(opts bootstrap.Opts, extensions Extensions) func() []plugins.Plugin {
	return GetPluginsWithExtensionsAndRegistry(opts, registry.Plugins, extensions)
}

func RunGloo(opts bootstrap.Opts) error {
	return RunGlooWithExtensions(opts, Extensions{})
}

func RunGlooWithExtensions(opts bootstrap.Opts, extensions Extensions) error {
	watchOpts := opts.WatchOpts.WithDefaults()
	opts.WatchOpts.Ctx = contextutils.WithLogger(opts.WatchOpts.Ctx, "gloo")

	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "setup")
	endpointsFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}

	upstreamClient, err := v1.NewUpstreamClient(opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}

	kubeServiceClient := opts.KubeServiceClient
	if opts.Settings.GetGloo().GetDisableKubernetesDestinations() {
		kubeServiceClient = nil
	}
	hybridUsClient, err := upstreams.NewHybridUpstreamClient(upstreamClient, kubeServiceClient, opts.Consul.ConsulWatcher)
	if err != nil {
		return err
	}

	proxyClient, err := v1.NewProxyClient(opts.Proxies)
	if err != nil {
		return err
	}
	if err := proxyClient.Register(); err != nil {
		return err
	}

	upstreamGroupClient, err := v1.NewUpstreamGroupClient(opts.UpstreamGroups)
	if err != nil {
		return err
	}
	if err := upstreamGroupClient.Register(); err != nil {
		return err
	}

	endpointClient, err := v1.NewEndpointClient(endpointsFactory)
	if err != nil {
		return err
	}

	secretClient, err := v1.NewSecretClient(opts.Secrets)
	if err != nil {
		return err
	}

	artifactClient, err := v1.NewArtifactClient(opts.Artifacts)
	if err != nil {
		return err
	}

	authConfigClient, err := extauth.NewAuthConfigClient(opts.AuthConfigs)
	if err != nil {
		return err
	}
	if err := authConfigClient.Register(); err != nil {
		return err
	}

	rlClient, rlReporterClient, err := rlv1alpha1.NewRateLimitClients(opts.RateLimitConfigs)
	if err != nil {
		return err
	}
	if err := rlClient.Register(); err != nil {
		return err
	}

	// Register grpc endpoints to the grpc server
	xds.SetupEnvoyXds(opts.ControlPlane.GrpcServer, opts.ControlPlane.XDSServer, opts.ControlPlane.SnapshotCache)
	xdsHasher := xds.NewNodeHasher()
	getPlugins := GetPluginsWithExtensions(opts, extensions)
	var discoveryPlugins []discovery.DiscoveryPlugin
	for _, plug := range getPlugins() {
		disc, ok := plug.(discovery.DiscoveryPlugin)
		if ok {
			discoveryPlugins = append(discoveryPlugins, disc)
		}
	}
	logger := contextutils.LoggerFrom(watchOpts.Ctx)

	errs := make(chan error)

	disc := discovery.NewEndpointDiscovery(opts.WatchNamespaces, opts.WriteNamespace, endpointClient, discoveryPlugins)
	edsSync := discovery.NewEdsSyncer(disc, discovery.Opts{}, watchOpts.RefreshRate)
	discoveryCache := v1.NewEdsEmitter(hybridUsClient)
	edsEventLoop := v1.NewEdsEventLoop(discoveryCache, edsSync)
	edsErrs, err := edsEventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}

	warmTimeout := opts.Settings.GetGloo().GetEndpointsWarmingTimeout()

	if warmTimeout == nil {
		warmTimeout = &types.Duration{
			Seconds: 5 * 60,
		}
	}
	if warmTimeout.Seconds != 0 || warmTimeout.Nanos != 0 {
		warmTimeoutDuration, err := types.DurationFromProto(warmTimeout)
		ctx := opts.WatchOpts.Ctx
		err = channelutils.WaitForReady(ctx, warmTimeoutDuration, edsEventLoop.Ready(), disc.Ready())
		if err != nil {
			// make sure that the reason we got here is not context cancellation
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Panicw("failed warming up endpoints - consider adjusting endpointsWarmingTimeout", "warmTimeoutDuration", warmTimeoutDuration)
		}
	}

	// We are ready!

	go errutils.AggregateErrs(watchOpts.Ctx, errs, edsErrs, "eds.gloo")

	apiCache := v1.NewApiEmitter(
		artifactClient,
		endpointClient,
		proxyClient,
		upstreamGroupClient,
		secretClient,
		hybridUsClient,
		authConfigClient,
		rlClient,
	)

	rpt := reporter.NewReporter("gloo",
		hybridUsClient.BaseClient(),
		proxyClient.BaseClient(),
		upstreamGroupClient.BaseClient(),
		authConfigClient.BaseClient(),
		rlReporterClient,
	)

	t := translator.NewTranslator(sslutils.NewSslConfigTranslator(), opts.Settings, getPlugins)

	validator := validation.NewValidator(watchOpts.Ctx, t)
	if opts.ValidationServer.Server != nil {
		opts.ValidationServer.Server.SetValidator(validator)
	}

	routeReplacingSanitizer, err := sanitizer.NewRouteReplacingSanitizer(opts.Settings.GetGloo().GetInvalidConfigPolicy())
	if err != nil {
		return err
	}

	xdsSanitizer := sanitizer.XdsSanitizers{
		sanitizer.NewUpstreamRemovingSanitizer(),
		routeReplacingSanitizer,
	}

	// Set up the syncer extension
	var syncerExtensions []TranslatorSyncerExtension
	params := TranslatorSyncerExtensionParams{
		Reporter: rpt,
		RateLimitServiceSettings: ratelimit.ServiceSettings{
			Descriptors: opts.Settings.GetRatelimit().GetDescriptors(),
		},
	}
	for _, syncerExtensionFactory := range extensions.SyncerExtensions {
		syncerExtension, err := syncerExtensionFactory(watchOpts.Ctx, params)
		if err != nil {
			logger.Errorw("Error initializing extension", "error", err)
			continue
		}
		syncerExtensions = append(syncerExtensions, syncerExtension)
	}

	translationSync := NewTranslatorSyncer(t, opts.ControlPlane.SnapshotCache, xdsHasher, xdsSanitizer, rpt, opts.DevMode, syncerExtensions, opts.Settings)

	syncers := v1.ApiSyncers{
		translationSync,
		validator,
	}

	apiEventLoop := v1.NewApiEventLoop(apiCache, syncers)
	apiEventLoopErrs, err := apiEventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, apiEventLoopErrs, "event_loop.gloo")

	go func() {
		for {
			select {
			case <-watchOpts.Ctx.Done():
				logger.Debugf("context cancelled")
				return
			}
		}
	}()

	if opts.ControlPlane.StartGrpcServer {
		// copy for the go-routines
		controlPlane := opts.ControlPlane
		lis, err := net.Listen(opts.ControlPlane.BindAddr.Network(), opts.ControlPlane.BindAddr.String())
		if err != nil {
			return err
		}
		go func() {
			<-controlPlane.GrpcService.Ctx.Done()
			controlPlane.GrpcServer.Stop()
		}()

		go func() {
			if err := controlPlane.GrpcServer.Serve(lis); err != nil {
				logger.Errorf("xds grpc server failed to start")
			}
		}()
		opts.ControlPlane.StartGrpcServer = false
	}

	if opts.ValidationServer.StartGrpcServer {
		validationServer := opts.ValidationServer
		lis, err := net.Listen(validationServer.BindAddr.Network(), validationServer.BindAddr.String())
		if err != nil {
			return err
		}
		validationServer.Server.Register(validationServer.GrpcServer)

		go func() {
			<-validationServer.Ctx.Done()
			validationServer.GrpcServer.Stop()
		}()

		go func() {
			if err := validationServer.GrpcServer.Serve(lis); err != nil {
				logger.Errorf("validation grpc server failed to start")
			}
		}()
		opts.ValidationServer.StartGrpcServer = false
	}

	go func() {
		var handler metricsservice.MetricsHandler

		if extensions.MetricsHandler == nil {
			handler, err = metricsservice.NewConfigMapBackedDefaultHandler(opts.WatchOpts.Ctx)
			if err != nil {
				contextutils.LoggerFrom(opts.WatchOpts.Ctx).Errorw("Error starting metrics watcher", zap.Error(err))
				return
			}
		} else {
			handler = extensions.MetricsHandler
		}

		if err := runner.RunE(opts.WatchOpts.Ctx, handler); err != nil {
			contextutils.LoggerFrom(opts.WatchOpts.Ctx).Errorw("err in metrics server", zap.Error(err))
		}
	}()

	go func() {
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					return
				}
				logger.Errorw("gloo main event loop", zap.Error(err))
			case <-opts.WatchOpts.Ctx.Done():
				// think about closing this channel
				// close(errs)
				return
			}
		}
	}()

	return nil
}

func constructOpts(ctx context.Context, clientset *kubernetes.Interface, kubeCache kube.SharedCache, consulClient *consulapi.Client, vaultClient *vaultapi.Client, memCache memory.InMemoryResourceCache, settings *v1.Settings) (bootstrap.Opts, error) {

	var (
		cfg           *rest.Config
		kubeCoreCache corecache.KubeCoreCache
	)

	params := bootstrap.NewConfigFactoryParams(
		settings,
		memCache,
		kubeCache,
		&cfg,
		consulClient,
	)

	upstreamFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.UpstreamCrd)
	if err != nil {
		return bootstrap.Opts{}, errors.Wrapf(err, "creating config source from settings")
	}

	kubeServiceClient, err := bootstrap.KubeServiceClientForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		clientset,
		&kubeCoreCache,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	proxyFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.ProxyCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	secretFactory, err := bootstrap.SecretFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		clientset,
		&kubeCoreCache,
		vaultClient,
		v1.SecretCrd.Plural,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	upstreamGroupFactory, err := bootstrap.ConfigFactoryForSettings(params, v1.UpstreamGroupCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	artifactFactory, err := bootstrap.ArtifactFactoryForSettings(
		ctx,
		settings,
		memCache,
		&cfg,
		clientset,
		&kubeCoreCache,
		consulClient,
		v1.ArtifactCrd.Plural,
	)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	authConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, extauth.AuthConfigCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	rateLimitConfigFactory, err := bootstrap.ConfigFactoryForSettings(params, rlv1alpha1.RateLimitConfigCrd)
	if err != nil {
		return bootstrap.Opts{}, err
	}

	return bootstrap.Opts{
		Upstreams:         upstreamFactory,
		KubeServiceClient: kubeServiceClient,
		Proxies:           proxyFactory,
		UpstreamGroups:    upstreamGroupFactory,
		Secrets:           secretFactory,
		Artifacts:         artifactFactory,
		AuthConfigs:       authConfigFactory,
		RateLimitConfigs:  rateLimitConfigFactory,
		KubeCoreCache:     kubeCoreCache,
	}, nil
}
