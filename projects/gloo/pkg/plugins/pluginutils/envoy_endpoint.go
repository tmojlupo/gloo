package pluginutils

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
)

func EnvoySingleEndpointLoadAssignment(out *envoy_config_cluster_v3.Cluster, address string, port uint32) {
	out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: out.Name,
		Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
			{
				LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
					{
						HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
							Endpoint: EnvoyEndpoint(address, port),
						},
					},
				},
			},
		},
	}
}

func EnvoyEndpoint(address string, port uint32) *envoy_config_endpoint_v3.Endpoint {
	return &envoy_config_endpoint_v3.Endpoint{
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_config_core_v3.SocketAddress{
					Address: address,
					PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
	}
}
