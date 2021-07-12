package translator

import (
	"fmt"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"go.opencensus.io/trace"
)

func (t *translatorInstance) computeClusters(
	params plugins.Params,
	reports reporter.ResourceReports,
	upstreamRefKeyToEndpoints map[string][]*v1.Endpoint,
	proxy *v1.Proxy,
) ([]*envoy_config_cluster_v3.Cluster, map[*envoy_config_cluster_v3.Cluster]*v1.Upstream) {

	ctx, span := trace.StartSpan(params.Ctx, "gloo.translator.computeClusters")
	params.Ctx = ctx
	defer span.End()

	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_clusters")
	upstreamGroups := params.Snapshot.UpstreamGroups
	upstreams := params.Snapshot.Upstreams
	clusters := make([]*envoy_config_cluster_v3.Cluster, 0, len(upstreams))
	validateUpstreamLambdaFunctions(proxy, upstreams, upstreamGroups, reports)

	clusterToUpstreamMap := make(map[*envoy_config_cluster_v3.Cluster]*v1.Upstream)
	// snapshot contains both real and service-derived upstreams
	for _, upstream := range upstreams {

		cluster := t.computeCluster(params, upstream, upstreamRefKeyToEndpoints, reports)
		clusterToUpstreamMap[cluster] = upstream
		clusters = append(clusters, cluster)
	}

	return clusters, clusterToUpstreamMap
}

func (t *translatorInstance) computeCluster(
	params plugins.Params,
	upstream *v1.Upstream,
	upstreamRefKeyToEndpoints map[string][]*v1.Endpoint,
	reports reporter.ResourceReports,
) *envoy_config_cluster_v3.Cluster {
	params.Ctx = contextutils.WithLogger(params.Ctx, upstream.Metadata.Name)
	out := t.initializeCluster(upstream, upstreamRefKeyToEndpoints, reports, &params.Snapshot.Secrets)

	for _, plug := range t.plugins {
		upstreamPlugin, ok := plug.(plugins.UpstreamPlugin)
		if !ok {
			continue
		}

		if err := upstreamPlugin.ProcessUpstream(params, upstream, out); err != nil {
			reports.AddError(upstream, err)
		}
	}
	if err := validateCluster(out); err != nil {
		reports.AddError(upstream, eris.Wrapf(err, "cluster was configured improperly "+
			"by one or more plugins: %v", out))
	}
	return out
}

func (t *translatorInstance) initializeCluster(
	upstream *v1.Upstream,
	upstreamRefKeyToEndpoints map[string][]*v1.Endpoint,
	reports reporter.ResourceReports,
	secrets *v1.SecretList,
) *envoy_config_cluster_v3.Cluster {
	hcConfig, err := createHealthCheckConfig(upstream, secrets)
	if err != nil {
		reports.AddError(upstream, err)
	}
	detectCfg, err := createOutlierDetectionConfig(upstream)
	if err != nil {
		reports.AddError(upstream, err)
	}

	circuitBreakers := t.settings.GetGloo().GetCircuitBreakers()
	out := &envoy_config_cluster_v3.Cluster{
		Name:             UpstreamToClusterName(upstream.Metadata.Ref()),
		Metadata:         new(envoy_config_core_v3.Metadata),
		CircuitBreakers:  getCircuitBreakers(upstream.CircuitBreakers, circuitBreakers),
		LbSubsetConfig:   createLbConfig(upstream),
		HealthChecks:     hcConfig,
		OutlierDetection: detectCfg,
		// this field can be overridden by plugins
		ConnectTimeout:       ptypes.DurationProto(ClusterConnectionTimeout),
		Http2ProtocolOptions: getHttp2options(upstream),
	}

	if sslConfig := upstream.SslConfig; sslConfig != nil {
		applyDefaultsToUpstreamSslConfig(sslConfig, t.settings.GetUpstreamOptions())
		cfg, err := utils.NewSslConfigTranslator().ResolveUpstreamSslConfig(*secrets, sslConfig)
		if err != nil {
			reports.AddError(upstream, err)
		} else {
			out.TransportSocket = &envoy_config_core_v3.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(cfg)},
			}
		}
	}

	// set Type = EDS if we have endpoints for the upstream
	if eps, ok := upstreamRefKeyToEndpoints[upstream.Metadata.Ref().Key()]; ok && len(eps) > 0 {
		xds.SetEdsOnCluster(out, t.settings)
	}
	return out
}

var (
	DefaultHealthCheckTimeout  = &duration.Duration{Seconds: 5}
	DefaultHealthCheckInterval = prototime.DurationToProto(time.Millisecond * 100)
	DefaultThreshold           = &wrappers.UInt32Value{
		Value: 5,
	}

	NilFieldError = func(fieldName string) error {
		return eris.Errorf("The field %s cannot be nil", fieldName)
	}
)

func createHealthCheckConfig(upstream *v1.Upstream, secrets *v1.SecretList) ([]*envoy_config_core_v3.HealthCheck, error) {
	if upstream == nil {
		return nil, nil
	}
	result := make([]*envoy_config_core_v3.HealthCheck, 0, len(upstream.GetHealthChecks()))
	for i, hc := range upstream.GetHealthChecks() {
		// These values are required by envoy, but not explicitly
		if hc.HealthyThreshold == nil {
			return nil, NilFieldError(fmt.Sprintf("HealthCheck[%d].HealthyThreshold", i))
		}
		if hc.UnhealthyThreshold == nil {
			return nil, NilFieldError(fmt.Sprintf("HealthCheck[%d].UnhealthyThreshold", i))
		}
		if hc.GetHealthChecker() == nil {
			return nil, NilFieldError(fmt.Sprintf("HealthCheck[%d].HealthChecker", i))
		}
		converted, err := api_conversion.ToEnvoyHealthCheck(hc, secrets)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func createOutlierDetectionConfig(upstream *v1.Upstream) (*envoy_config_cluster_v3.OutlierDetection, error) {
	if upstream.GetOutlierDetection() == nil {
		return nil, nil
	}
	if upstream.GetOutlierDetection().GetInterval() == nil {
		return nil, NilFieldError("OutlierDetection.HealthChecker.Interval")
	}
	return api_conversion.ToEnvoyOutlierDetection(upstream.GetOutlierDetection()), nil
}

func createLbConfig(upstream *v1.Upstream) *envoy_config_cluster_v3.Cluster_LbSubsetConfig {
	specGetter, ok := upstream.GetUpstreamType().(v1.SubsetSpecGetter)
	if !ok {
		return nil
	}
	glooSubsetConfig := specGetter.GetSubsetSpec()
	if glooSubsetConfig == nil {
		return nil
	}

	subsetConfig := &envoy_config_cluster_v3.Cluster_LbSubsetConfig{
		FallbackPolicy: envoy_config_cluster_v3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
	}
	for _, keys := range glooSubsetConfig.GetSelectors() {
		subsetConfig.SubsetSelectors = append(subsetConfig.GetSubsetSelectors(), &envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
			Keys: keys.GetKeys(),
		})
	}

	return subsetConfig
}

// TODO: add more validation here
func validateCluster(c *envoy_config_cluster_v3.Cluster) error {
	if c.GetClusterType() != nil {
		// TODO(yuval-k): this is a custom cluster, we cant validate it for now.
		return nil
	}
	clusterType := c.GetType()
	if clusterType == envoy_config_cluster_v3.Cluster_STATIC ||
		clusterType == envoy_config_cluster_v3.Cluster_STRICT_DNS ||
		clusterType == envoy_config_cluster_v3.Cluster_LOGICAL_DNS {
		if len(c.GetLoadAssignment().GetEndpoints()) == 0 {
			return eris.Errorf("cluster type %v specified but LoadAssignment was empty", clusterType.String())
		}
	}
	return nil
}

// Convert the first non nil circuit breaker.
func getCircuitBreakers(cfgs ...*v1.CircuitBreakerConfig) *envoy_config_cluster_v3.CircuitBreakers {
	for _, cfg := range cfgs {
		if cfg != nil {
			envoyCfg := &envoy_config_cluster_v3.CircuitBreakers{}
			envoyCfg.Thresholds = []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{{
				MaxConnections:     cfg.GetMaxConnections(),
				MaxPendingRequests: cfg.GetMaxPendingRequests(),
				MaxRequests:        cfg.GetMaxRequests(),
				MaxRetries:         cfg.GetMaxRetries(),
			}}
			return envoyCfg
		}
	}
	return nil
}

func getHttp2options(us *v1.Upstream) *envoy_config_core_v3.Http2ProtocolOptions {
	if us.GetUseHttp2().GetValue() {
		return &envoy_config_core_v3.Http2ProtocolOptions{}
	}
	return nil
}

// Validates routes that point to the current AWS lambda upstream
// Checks that the function the route is pointing to is available on the upstream
// else it adds an error to the upstream, so that invalid route replacement can be used.
func validateUpstreamLambdaFunctions(proxy *v1.Proxy, upstreams v1.UpstreamList, upstreamGroups v1.UpstreamGroupList, reports reporter.ResourceReports) {
	// Create a set of the lambda functions in each upstream
	upstreamLambdas := make(map[string]map[string]bool)
	for _, upstream := range upstreams {
		lambdaFuncs := upstream.GetAws().GetLambdaFunctions()
		for _, lambda := range lambdaFuncs {
			upstreamRef := UpstreamToClusterName(upstream.Metadata.Ref())
			if upstreamLambdas[upstreamRef] == nil {
				upstreamLambdas[upstreamRef] = make(map[string]bool)
			}
			upstreamLambdas[upstreamRef][lambda.GetLogicalName()] = true
		}
	}

	for _, listener := range proxy.GetListeners() {
		httpListener := listener.GetHttpListener()
		if httpListener != nil {
			for _, virtualHost := range httpListener.GetVirtualHosts() {
				// Validate all routes to make sure that if they point to a lambda, it exists.
				for _, route := range virtualHost.GetRoutes() {
					validateRouteDestinationForValidLambdas(proxy, route, upstreamGroups, reports, upstreamLambdas)
				}
			}
		}
	}
}

// Validates a route that may have a single or multi upstream destinations to make sure that any lambda upstreams are referencing valid lambdas
func validateRouteDestinationForValidLambdas(
	proxy *v1.Proxy,
	route *v1.Route,
	upstreamGroups v1.UpstreamGroupList,
	reports reporter.ResourceReports,
	upstreamLambdas map[string]map[string]bool,
) {
	// Append destinations to a destination list to process all of them in one go
	var destinations []*v1.Destination

	_, ok := route.GetAction().(*v1.Route_RouteAction)
	if !ok {
		// If this is not a Route_RouteAction (e.g. Route_DirectResponseAction, Route_RedirectAction), there is no destination to validate
		return
	}
	routeAction := route.GetRouteAction()
	switch typedRoute := routeAction.GetDestination().(type) {
	case *v1.RouteAction_Single:
		{
			destinations = append(destinations, routeAction.GetSingle())
		}
	case *v1.RouteAction_Multi:
		{
			multiDest := routeAction.GetMulti()
			for _, weightedDest := range multiDest.GetDestinations() {
				destinations = append(destinations, weightedDest.GetDestination())
			}
		}
	case *v1.RouteAction_UpstreamGroup:
		{
			ugRef := typedRoute.UpstreamGroup
			ug, err := upstreamGroups.Find(ugRef.GetNamespace(), ugRef.GetName())
			if err != nil {
				reports.AddError(proxy, fmt.Errorf("upstream group not found, (Name: %s, Namespace: %s)", ugRef.GetName(), ugRef.GetNamespace()))
				return
			}
			for _, weightedDest := range ug.GetDestinations() {
				destinations = append(destinations, weightedDest.GetDestination())
			}
		}
	case *v1.RouteAction_ClusterHeader:
		{
			// no upstream configuration is provided in this case; can't validate the route
			return
		}
	default:
		{
			reports.AddError(proxy, fmt.Errorf("route destination type %T not supported with AWS Lambda", typedRoute))
			return
		}
	}

	// Process destinations (upstreams)
	for _, dest := range destinations {
		routeUpstream := dest.GetUpstream()
		// Check that route is pointing to current upstream
		if routeUpstream != nil {
			// Get the lambda functions that this upstream has
			lambdaFuncSet := upstreamLambdas[UpstreamToClusterName(routeUpstream)]
			routeLambda := dest.GetDestinationSpec().GetAws()
			routeLambdaName := routeLambda.GetLogicalName()
			// If route is pointing to a lambda that does not exist on this upstream, report error on the upstream
			if routeLambda != nil && lambdaFuncSet[routeLambdaName] == false {
				// Add error to the proxy which has the faulty route pointing to a non-existent lambda
				reports.AddError(proxy, fmt.Errorf("a route references %s AWS lambda which does not exist on the route's upstream", routeLambdaName))
			}
		}
	}

}

// Apply defaults to UpstreamSslConfig
func applyDefaultsToUpstreamSslConfig(sslConfig *v1.UpstreamSslConfig, options *v1.UpstreamOptions) {
	if options == nil {
		return
	}

	// Apply default SslParameters if none are defined on upstream
	if sslConfig.GetParameters() == nil {
		sslConfig.Parameters = options.GetSslParameters()
	}
}
