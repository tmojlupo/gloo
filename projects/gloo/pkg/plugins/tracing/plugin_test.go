package tracing

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytrace "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytracing "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	envoytrace_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	It("should update listener properly", func() {
		pluginParams := plugins.Params{
			Snapshot: nil,
		}
		p := NewPlugin()
		cfg := &envoyhttp.HttpConnectionManager{}
		hcmSettings := &hcm.HttpConnectionManagerSettings{
			Tracing: &tracing.ListenerTracingSettings{
				RequestHeadersForTags: []string{"header1", "header2"},
				EnvironmentVariablesForTags: []*tracing.TracingTagEnvironmentVariable{
					{
						Tag:  "k8s.pod.name",
						Name: "POD_NAME",
					},
					{
						Tag:          "k8s.pod.ip",
						Name:         "POD_IP",
						DefaultValue: "NO_POD_IP",
					},
				},
				LiteralsForTags: []*tracing.TracingTagLiteral{
					{
						Tag:   "foo",
						Value: "bar",
					},
				},
				Verbose: true,
				TracePercentages: &tracing.TracePercentages{
					ClientSamplePercentage:  &wrappers.FloatValue{Value: 10},
					RandomSamplePercentage:  &wrappers.FloatValue{Value: 20},
					OverallSamplePercentage: &wrappers.FloatValue{Value: 30},
				},
				ProviderConfig: nil,
			},
		}
		err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
		Expect(err).To(BeNil())
		expected := &envoyhttp.HttpConnectionManager{
			Tracing: &envoyhttp.HttpConnectionManager_Tracing{
				CustomTags: []*envoytracing.CustomTag{
					{
						Tag: "header1",
						Type: &envoytracing.CustomTag_RequestHeader{
							RequestHeader: &envoytracing.CustomTag_Header{
								Name: "header1",
							},
						},
					},
					{
						Tag: "header2",
						Type: &envoytracing.CustomTag_RequestHeader{
							RequestHeader: &envoytracing.CustomTag_Header{
								Name: "header2",
							},
						},
					},
					{
						Tag: "k8s.pod.name",
						Type: &envoytracing.CustomTag_Environment_{
							Environment: &envoytracing.CustomTag_Environment{
								Name: "POD_NAME",
							},
						},
					},
					{
						Tag: "k8s.pod.ip",
						Type: &envoytracing.CustomTag_Environment_{
							Environment: &envoytracing.CustomTag_Environment{
								Name:         "POD_IP",
								DefaultValue: "NO_POD_IP",
							},
						},
					},
					{
						Tag: "foo",
						Type: &envoytracing.CustomTag_Literal_{
							Literal: &envoytracing.CustomTag_Literal{
								Value: "bar",
							},
						},
					},
				},
				ClientSampling:  &envoy_type.Percent{Value: 10},
				RandomSampling:  &envoy_type.Percent{Value: 20},
				OverallSampling: &envoy_type.Percent{Value: 30},
				Verbose:         true,
				Provider:        nil,
			},
		}
		Expect(cfg).To(Equal(expected))
	})

	It("should update listener properly - with defaults", func() {
		pluginParams := plugins.Params{
			Snapshot: nil,
		}
		p := NewPlugin()
		cfg := &envoyhttp.HttpConnectionManager{}
		hcmSettings := &hcm.HttpConnectionManagerSettings{
			Tracing: &tracing.ListenerTracingSettings{},
		}
		err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
		Expect(err).To(BeNil())
		expected := &envoyhttp.HttpConnectionManager{
			Tracing: &envoyhttp.HttpConnectionManager_Tracing{
				ClientSampling:  &envoy_type.Percent{Value: 100},
				RandomSampling:  &envoy_type.Percent{Value: 100},
				OverallSampling: &envoy_type.Percent{Value: 100},
				Verbose:         false,
				Provider:        nil,
			},
		}
		Expect(cfg).To(Equal(expected))
	})

	Context("should handle tracing provider config", func() {

		It("when provider config is nil", func() {
			pluginParams := plugins.Params{
				Snapshot: nil,
			}
			p := NewPlugin()
			cfg := &envoyhttp.HttpConnectionManager{}
			hcmSettings := &hcm.HttpConnectionManagerSettings{
				Tracing: &tracing.ListenerTracingSettings{
					ProviderConfig: nil,
				},
			}
			err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
			Expect(err).To(BeNil())
			Expect(cfg.Tracing.Provider).To(BeNil())
		})

		Describe("when zipkin provider config", func() {
			It("references invalid upstream", func() {
				pluginParams := plugins.Params{
					Snapshot: &v1.ApiSnapshot{
						Upstreams: v1.UpstreamList{
							// No valid upstreams
						},
					},
				}
				p := NewPlugin()
				cfg := &envoyhttp.HttpConnectionManager{}
				hcmSettings := &hcm.HttpConnectionManagerSettings{
					Tracing: &tracing.ListenerTracingSettings{
						ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
							ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
								CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
									CollectorUpstreamRef: &core.ResourceRef{
										Name:      "invalid-name",
										Namespace: "invalid-namespace",
									},
								},
							},
						},
					},
				}
				err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
				Expect(err).NotTo(BeNil())
			})

			It("references valid upstream", func() {
				us := v1.NewUpstream("default", "valid")
				pluginParams := plugins.Params{
					Snapshot: &v1.ApiSnapshot{
						Upstreams: v1.UpstreamList{us},
					},
				}
				p := NewPlugin()
				cfg := &envoyhttp.HttpConnectionManager{}
				hcmSettings := &hcm.HttpConnectionManagerSettings{
					Tracing: &tracing.ListenerTracingSettings{
						ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
							ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
								CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
									CollectorUpstreamRef: &core.ResourceRef{
										Name:      "valid",
										Namespace: "default",
									},
								},
								CollectorEndpoint:        "/api/v2/spans",
								CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
								SharedSpanContext:        nil,
								TraceId_128Bit:           false,
							},
						},
					},
				}
				err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
				Expect(err).To(BeNil())

				expectedEnvoyConfig := &envoytrace.ZipkinConfig{
					CollectorCluster:         "valid_default",
					CollectorEndpoint:        "/api/v2/spans",
					CollectorEndpointVersion: envoytrace.ZipkinConfig_HTTP_JSON,
					SharedSpanContext:        nil,
					TraceId_128Bit:           false,
				}
				expectedEnvoyConfigMarshalled, _ := ptypes.MarshalAny(expectedEnvoyConfig)

				expectedEnvoyTracingProvider := &envoytrace.Tracing_Http{
					Name: "envoy.tracers.zipkin",
					ConfigType: &envoytrace.Tracing_Http_TypedConfig{
						TypedConfig: expectedEnvoyConfigMarshalled,
					},
				}

				Expect(cfg.Tracing.Provider.GetName()).To(Equal(expectedEnvoyTracingProvider.GetName()))
				Expect(cfg.Tracing.Provider.GetTypedConfig()).To(Equal(expectedEnvoyTracingProvider.GetTypedConfig()))
			})

			It("references cluster name", func() {
				pluginParams := plugins.Params{}
				p := NewPlugin()
				cfg := &envoyhttp.HttpConnectionManager{}
				hcmSettings := &hcm.HttpConnectionManagerSettings{
					Tracing: &tracing.ListenerTracingSettings{
						ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
							ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
								CollectorCluster: &envoytrace_gloo.ZipkinConfig_ClusterName{
									ClusterName: "zipkin-cluster-name",
								},
								CollectorEndpoint:        "/api/v2/spans",
								CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
								SharedSpanContext:        nil,
								TraceId_128Bit:           false,
							},
						},
					},
				}
				err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
				Expect(err).To(BeNil())

				expectedEnvoyConfig := &envoytrace.ZipkinConfig{
					CollectorCluster:         "zipkin-cluster-name",
					CollectorEndpoint:        "/api/v2/spans",
					CollectorEndpointVersion: envoytrace.ZipkinConfig_HTTP_JSON,
					SharedSpanContext:        nil,
					TraceId_128Bit:           false,
				}
				expectedEnvoyConfigMarshalled, _ := ptypes.MarshalAny(expectedEnvoyConfig)

				expectedEnvoyTracingProvider := &envoytrace.Tracing_Http{
					Name: "envoy.tracers.zipkin",
					ConfigType: &envoytrace.Tracing_Http_TypedConfig{
						TypedConfig: expectedEnvoyConfigMarshalled,
					},
				}

				Expect(cfg.Tracing.Provider.GetName()).To(Equal(expectedEnvoyTracingProvider.GetName()))
				Expect(cfg.Tracing.Provider.GetTypedConfig()).To(Equal(expectedEnvoyTracingProvider.GetTypedConfig()))
			})
		})

		Describe("when datadog provider config", func() {
			It("references invalid upstream", func() {
				pluginParams := plugins.Params{
					Snapshot: &v1.ApiSnapshot{
						Upstreams: v1.UpstreamList{
							// No valid upstreams
						},
					},
				}
				p := NewPlugin()
				cfg := &envoyhttp.HttpConnectionManager{}
				hcmSettings := &hcm.HttpConnectionManagerSettings{
					Tracing: &tracing.ListenerTracingSettings{
						ProviderConfig: &tracing.ListenerTracingSettings_DatadogConfig{
							DatadogConfig: &envoytrace_gloo.DatadogConfig{
								CollectorCluster: &envoytrace_gloo.DatadogConfig_CollectorUpstreamRef{
									CollectorUpstreamRef: &core.ResourceRef{
										Name:      "invalid-name",
										Namespace: "invalid-namespace",
									},
								},
							},
						},
					},
				}
				err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
				Expect(err).NotTo(BeNil())
			})

			It("references valid upstream", func() {
				us := v1.NewUpstream("default", "valid")
				pluginParams := plugins.Params{
					Snapshot: &v1.ApiSnapshot{
						Upstreams: v1.UpstreamList{us},
					},
				}
				p := NewPlugin()
				cfg := &envoyhttp.HttpConnectionManager{}
				hcmSettings := &hcm.HttpConnectionManagerSettings{
					Tracing: &tracing.ListenerTracingSettings{
						ProviderConfig: &tracing.ListenerTracingSettings_DatadogConfig{
							DatadogConfig: &envoytrace_gloo.DatadogConfig{
								CollectorCluster: &envoytrace_gloo.DatadogConfig_CollectorUpstreamRef{
									CollectorUpstreamRef: &core.ResourceRef{
										Name:      "valid",
										Namespace: "default",
									},
								},
								ServiceName: "datadog-gloo",
							},
						},
					},
				}
				err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
				Expect(err).To(BeNil())

				expectedEnvoyConfig := &envoytrace.DatadogConfig{
					CollectorCluster: "valid_default",
					ServiceName:      "datadog-gloo",
				}
				expectedEnvoyConfigMarshalled, _ := ptypes.MarshalAny(expectedEnvoyConfig)

				expectedEnvoyTracingProvider := &envoytrace.Tracing_Http{
					Name: "envoy.tracers.datadog",
					ConfigType: &envoytrace.Tracing_Http_TypedConfig{
						TypedConfig: expectedEnvoyConfigMarshalled,
					},
				}

				Expect(cfg.Tracing.Provider.GetName()).To(Equal(expectedEnvoyTracingProvider.GetName()))
				Expect(cfg.Tracing.Provider.GetTypedConfig()).To(Equal(expectedEnvoyTracingProvider.GetTypedConfig()))
			})

			It("references cluster name", func() {
				pluginParams := plugins.Params{}
				p := NewPlugin()
				cfg := &envoyhttp.HttpConnectionManager{}
				hcmSettings := &hcm.HttpConnectionManagerSettings{
					Tracing: &tracing.ListenerTracingSettings{
						ProviderConfig: &tracing.ListenerTracingSettings_DatadogConfig{
							DatadogConfig: &envoytrace_gloo.DatadogConfig{
								CollectorCluster: &envoytrace_gloo.DatadogConfig_ClusterName{
									ClusterName: "datadog-cluster-name",
								},
								ServiceName: "datadog-gloo",
							},
						},
					},
				}
				err := p.ProcessHcmSettings(pluginParams.Snapshot, cfg, hcmSettings)
				Expect(err).To(BeNil())

				expectedEnvoyConfig := &envoytrace.DatadogConfig{
					CollectorCluster: "datadog-cluster-name",
					ServiceName:      "datadog-gloo",
				}
				expectedEnvoyConfigMarshalled, _ := ptypes.MarshalAny(expectedEnvoyConfig)

				expectedEnvoyTracingProvider := &envoytrace.Tracing_Http{
					Name: "envoy.tracers.datadog",
					ConfigType: &envoytrace.Tracing_Http_TypedConfig{
						TypedConfig: expectedEnvoyConfigMarshalled,
					},
				}

				Expect(cfg.Tracing.Provider.GetName()).To(Equal(expectedEnvoyTracingProvider.GetName()))
				Expect(cfg.Tracing.Provider.GetTypedConfig()).To(Equal(expectedEnvoyTracingProvider.GetTypedConfig()))
			})
		})

	})

	It("should update routes properly", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())

		inFull := &v1.Route{
			Options: &v1.RouteOptions{
				Tracing: &tracing.RouteTracingSettings{
					RouteDescriptor: "hello",
					Propagate:       &wrappers.BoolValue{Value: false},
				},
			},
		}
		outFull := &envoy_config_route_v3.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, inFull, outFull)
		Expect(err).NotTo(HaveOccurred())
		Expect(outFull.Decorator.Operation).To(Equal("hello"))
		Expect(outFull.Decorator.Propagate).To(Equal(&wrappers.BoolValue{Value: false}))
		Expect(outFull.Tracing.ClientSampling.Numerator / 10000).To(Equal(uint32(100)))
		Expect(outFull.Tracing.RandomSampling.Numerator / 10000).To(Equal(uint32(100)))
		Expect(outFull.Tracing.OverallSampling.Numerator / 10000).To(Equal(uint32(100)))
	})

	It("should update routes properly - with defaults", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())

		inFull := &v1.Route{
			Options: &v1.RouteOptions{
				Tracing: &tracing.RouteTracingSettings{
					RouteDescriptor: "hello",
					TracePercentages: &tracing.TracePercentages{
						ClientSamplePercentage:  &wrappers.FloatValue{Value: 10},
						RandomSamplePercentage:  &wrappers.FloatValue{Value: 20},
						OverallSamplePercentage: &wrappers.FloatValue{Value: 30},
					},
				},
			},
		}
		outFull := &envoy_config_route_v3.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, inFull, outFull)
		Expect(err).NotTo(HaveOccurred())
		Expect(outFull.Decorator.Operation).To(Equal("hello"))
		Expect(outFull.Decorator.Propagate).To(BeNil())
		Expect(outFull.Tracing.ClientSampling.Numerator / 10000).To(Equal(uint32(10)))
		Expect(outFull.Tracing.RandomSampling.Numerator / 10000).To(Equal(uint32(20)))
		Expect(outFull.Tracing.OverallSampling.Numerator / 10000).To(Equal(uint32(30)))
	})

})
