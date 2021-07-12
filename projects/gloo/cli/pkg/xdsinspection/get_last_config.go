package xdsinspection

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/rotisserie/eris"
	_ "github.com/solo-io/gloo/hack/filter_types"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"sigs.k8s.io/yaml"
)

const (
	envoySidecarConfig = "envoy-sidecar-config"
)

func GetGlooXdsDump(ctx context.Context, proxyName, namespace string, verboseErrors bool) (*XdsDump, error) {

	xdsPort := strconv.Itoa(int(defaults.GlooXdsPort))
	// If gloo is in MTLS mode
	glooMtlsCheck := exec.Command("kubectl", "get", "configmap", envoySidecarConfig, "-n", namespace)
	if err := glooMtlsCheck.Run(); err == nil {
		xdsPort = strconv.Itoa(int(defaults.GlooMtlsModeXdsPort))
	}
	portFwd := exec.Command("kubectl", "port-forward", "-n", namespace,
		"deployment/gloo", xdsPort)
	mergedPortForwardOutput := bytes.NewBuffer([]byte{})
	portFwd.Stdout = mergedPortForwardOutput
	portFwd.Stderr = mergedPortForwardOutput
	if err := portFwd.Start(); err != nil {
		return nil, eris.Wrapf(err, "failed to start port-forward")
	}
	defer func() {
		if portFwd.Process != nil {
			portFwd.Process.Kill()
		}
	}()
	result := make(chan *XdsDump)
	errs := make(chan error)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(time.Millisecond * 250)
			out, err := getXdsDump(ctx, xdsPort, proxyName, namespace)
			if err != nil {
				errs <- err
				continue
			}
			result <- out
			return
		}
	}()

	timer := time.Tick(time.Second * 5)

	for {
		select {
		case <-ctx.Done():
			return nil, eris.Errorf("cancelled")
		case err := <-errs:
			if verboseErrors {
				contextutils.LoggerFrom(ctx).Errorf("connecting to gloo failed with err %v", err.Error())
			}
		case res := <-result:
			return res, nil
		case <-timer:
			contextutils.LoggerFrom(ctx).Errorf("connecting to gloo failed with err %v",
				zap.Any("cmdErrors", string(mergedPortForwardOutput.Bytes())))
			return nil, eris.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}

type XdsDump struct {
	Role      string
	Endpoints []envoyendpoint.ClusterLoadAssignment
	Clusters  []envoycluster.Cluster
	Listeners []envoylistener.Listener
	Routes    []envoy_config_route_v3.RouteConfiguration
}

func getXdsDump(ctx context.Context, xdsPort, proxyName, proxyNamespace string) (*XdsDump, error) {
	xdsDump := &XdsDump{
		Role: fmt.Sprintf("%v~%v", proxyNamespace, proxyName),
	}
	dr := &discovery_v3.DiscoveryRequest{Node: &envoycore.Node{
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{"role": {Kind: &structpb.Value_StringValue{StringValue: xdsDump.Role}}}},
	}}

	conn, err := grpc.Dial("localhost:"+xdsPort, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	xdsDump.Endpoints, err = listEndpoints(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	xdsDump.Clusters, err = listClusters(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	xdsDump.Listeners, err = listListeners(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	// dig through hcms in listeners to find routes
	var hcms []envoyhttp.HttpConnectionManager
	for _, l := range xdsDump.Listeners {
		for _, fc := range l.FilterChains {
			for _, filter := range fc.Filters {
				if filter.Name == "envoy.http_connection_manager" {
					var hcm envoyhttp.HttpConnectionManager
					switch config := filter.ConfigType.(type) {
					case *envoylistener.Filter_TypedConfig:
						if err = ptypes.UnmarshalAny(config.TypedConfig, &hcm); err == nil {
							hcms = append(hcms, hcm)
						}
					case *envoylistener.Filter_HiddenEnvoyDeprecatedConfig:
						if err = envoyutil.StructToMessage(config.HiddenEnvoyDeprecatedConfig, &hcm); err == nil {
							hcms = append(hcms, hcm)
						}
					}
				}
			}
		}
	}

	var routes []string
	for _, hcm := range hcms {
		routes = append(routes, hcm.RouteSpecifier.(*envoyhttp.HttpConnectionManager_Rds).Rds.RouteConfigName)
	}

	xdsDump.Routes, err = listRoutes(ctx, conn, dr, routes)
	if err != nil {
		return nil, err
	}

	return xdsDump, nil
}

func listClusters(ctx context.Context, dr *discovery_v3.DiscoveryRequest, conn *grpc.ClientConn) ([]envoycluster.Cluster, error) {

	// clusters
	cdsc := envoy_service_cluster_v3.NewClusterDiscoveryServiceClient(conn)
	dresp, err := cdsc.FetchClusters(ctx, dr)
	if err != nil {
		return nil, err
	}
	var clusters []envoycluster.Cluster
	for _, anyCluster := range dresp.Resources {

		var cluster envoycluster.Cluster
		if err := ptypes.UnmarshalAny(anyCluster, &cluster); err != nil {
			return nil, err
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

func listEndpoints(ctx context.Context, dr *discovery_v3.DiscoveryRequest, conn *grpc.ClientConn) ([]envoyendpoint.ClusterLoadAssignment, error) {
	eds := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(conn)
	dresp, err := eds.FetchEndpoints(ctx, dr)
	if err != nil {
		return nil, eris.Errorf("endpoints err: %v", err)
	}
	var class []envoyendpoint.ClusterLoadAssignment

	for _, anyCla := range dresp.Resources {

		var cla envoyendpoint.ClusterLoadAssignment
		if err := ptypes.UnmarshalAny(anyCla, &cla); err != nil {
			return nil, err
		}
		class = append(class, cla)
	}
	return class, nil
}

func listListeners(ctx context.Context, dr *discovery_v3.DiscoveryRequest, conn *grpc.ClientConn) ([]envoylistener.Listener, error) {

	// listeners
	ldsc := envoy_service_listener_v3.NewListenerDiscoveryServiceClient(conn)
	dresp, err := ldsc.FetchListeners(ctx, dr)
	if err != nil {
		return nil, eris.Errorf("listeners err: %v", err)
	}
	var listeners []envoylistener.Listener

	for _, anylistener := range dresp.Resources {
		var listener envoylistener.Listener
		if err := ptypes.UnmarshalAny(anylistener, &listener); err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func listRoutes(ctx context.Context, conn *grpc.ClientConn, dr *discovery_v3.DiscoveryRequest, routenames []string) ([]envoy_config_route_v3.RouteConfiguration, error) {

	// routes
	ldsc := envoy_service_route_v3.NewRouteDiscoveryServiceClient(conn)

	dr.ResourceNames = routenames

	dresp, err := ldsc.FetchRoutes(ctx, dr)
	if err != nil {
		return nil, eris.Errorf("routes err: %v", err)
	}
	var routes []envoy_config_route_v3.RouteConfiguration

	for _, anyRoute := range dresp.Resources {
		var route envoy_config_route_v3.RouteConfiguration
		if err := ptypes.UnmarshalAny(anyRoute, &route); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (xd *XdsDump) String() string {
	buf := &bytes.Buffer{}
	errString := "unable to parse yaml"

	fmt.Fprintf(buf, "\n\n#role: %v", xd.Role)
	fmt.Fprintf(buf, "\n\n#clusters")
	for _, c := range xd.Clusters {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}

	fmt.Fprintf(buf, "\n\n#eds")
	for _, c := range xd.Endpoints {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}

	fmt.Fprintf(buf, "\n\n#listeners")
	for _, c := range xd.Listeners {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}
	fmt.Fprintf(buf, "\n\n#rds")

	for _, c := range xd.Routes {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}

	buf.WriteString("\n")
	return buf.String()
}

func toYaml(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	jpb := &jsonpb.Marshaler{}
	err := jpb.Marshal(buf, pb)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(buf.Bytes())
}
