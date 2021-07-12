package translator_test

import (
	"context"
	"fmt"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/projects/gloo/constants"
	gloo_envoy_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	v1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	consul2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	v1kubernetes "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	. "github.com/solo-io/solo-kit/test/matchers"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Translator", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		upstream          *v1.Upstream
		upName            *core.Metadata
		proxy             *v1.Proxy
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		matcher           *matchers.Matcher
		routes            []*v1.Route

		snapshot           envoycache.Snapshot
		cluster            *envoy_config_cluster_v3.Cluster
		listener           *envoy_config_listener_v3.Listener
		endpoints          envoycache.Resources
		hcmCfg             *envoyhttp.HttpConnectionManager
		routeConfiguration *envoy_config_route_v3.RouteConfiguration
		virtualHostName    string
	)

	beforeEach := func() {

		ctrl = gomock.NewController(T)

		cluster = nil
		settings = &v1.Settings{}
		memoryClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		opts := bootstrap.Opts{
			Settings:  settings,
			Secrets:   memoryClientFactory,
			Upstreams: memoryClientFactory,
			Consul: bootstrap.Consul{
				ConsulWatcher: mock_consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
			},
		}
		registeredPlugins = registry.Plugins(opts)

		upName = &core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		}
		upstream = &v1.Upstream{
			Metadata: upName,
			UpstreamType: &v1.Upstream_Static{
				Static: &v1static.UpstreamSpec{
					Hosts: []*v1static.Host{
						{
							Addr: "Test",
							Port: 124,
						},
					},
				},
			},
		}

		params = plugins.Params{
			Ctx: context.Background(),
			Snapshot: &v1.ApiSnapshot{
				Endpoints: v1.EndpointList{
					{
						Upstreams: []*core.ResourceRef{upName.Ref()},
						Address:   "1.2.3.4",
						Port:      32,
						Metadata: &core.Metadata{
							Name:      "test-ep",
							Namespace: "gloo-system",
						},
					},
				},
				Upstreams: v1.UpstreamList{
					upstream,
				},
			},
		}
		matcher = &matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: "/",
			},
		}
		routes = []*v1.Route{{
			Name:     "testRouteName",
			Matchers: []*matchers.Matcher{matcher},
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: upName.Ref(),
							},
						},
					},
				},
			},
		}}
		virtualHostName = "virt1"
	}
	BeforeEach(beforeEach)

	JustBeforeEach(func() {
		getPlugins := func() []plugins.Plugin {
			return registeredPlugins
		}
		translator = NewTranslator(glooutils.NewSslConfigTranslator(), settings, getPlugins)
		httpListener := &v1.Listener{
			Name:        "http-listener",
			BindAddress: "127.0.0.1",
			BindPort:    80,
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					VirtualHosts: []*v1.VirtualHost{{
						Name:    virtualHostName,
						Domains: []string{"*"},
						Routes:  routes,
					}},
				},
			},
		}
		tcpListener := &v1.Listener{
			Name:        "tcp-listener",
			BindAddress: "127.0.0.1",
			BindPort:    8080,
			ListenerType: &v1.Listener_TcpListener{
				TcpListener: &v1.TcpListener{
					TcpHosts: []*v1.TcpHost{
						{
							Destination: &v1.TcpHost_TcpAction{
								Destination: &v1.TcpHost_TcpAction_Single{
									Single: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "test",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
							SslConfig: &v1.SslConfig{
								SslSecrets: &v1.SslConfig_SslFiles{
									SslFiles: &v1.SSLFiles{
										TlsCert: "cert1",
										TlsKey:  "key1",
									},
								},
								SniDomains: []string{
									"sni1",
								},
							},
						},
					},
				},
			},
		}
		proxy = &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "test",
				Namespace: "gloo-system",
			},
			Listeners: []*v1.Listener{
				httpListener,
				tcpListener,
			},
		}
	})

	translateWithError := func() *validation.ProxyReport {
		_, errs, report, err := translator.Translate(params, proxy)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, errs.Validate()).To(HaveOccurred())
		return report
	}

	// returns md5 Sum of current snapshot
	translate := func() {
		snap, errs, report, err := translator.Translate(params, proxy)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, errs.Validate()).NotTo(HaveOccurred())
		ExpectWithOffset(1, snap).NotTo(BeNil())
		ExpectWithOffset(1, report).To(Equal(validationutils.MakeReport(proxy)))

		clusters := snap.GetResources(resource.ClusterTypeV3)
		clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
		cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
		ExpectWithOffset(1, cluster).NotTo(BeNil())

		listeners := snap.GetResources(resource.ListenerTypeV3)
		listenerResource := listeners.Items["http-listener"]
		listener = listenerResource.ResourceProto().(*envoy_config_listener_v3.Listener)
		ExpectWithOffset(1, listener).NotTo(BeNil())

		hcmFilter := listener.FilterChains[0].Filters[0]
		hcmCfg = &envoyhttp.HttpConnectionManager{}
		err = ParseTypedConfig(hcmFilter, hcmCfg)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		routes := snap.GetResources(resource.RouteTypeV3)
		ExpectWithOffset(1, routes.Items).To(HaveKey("http-listener-routes"))
		routeResource := routes.Items["http-listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		ExpectWithOffset(1, routeConfiguration).NotTo(BeNil())

		endpoints = snap.GetResources(resource.EndpointTypeV3)

		snapshot = snap

	}

	It("sanitizes an invalid virtual host name", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)
		proxyClone.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Name = "invalid.name"

		snap, errs, report, err := translator.Translate(params, proxyClone)

		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		routes := snap.GetResources(resource.RouteTypeV3)
		Expect(routes.Items).To(HaveKey("http-listener-routes"))
		routeResource := routes.Items["http-listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())
		Expect(routeConfiguration.GetVirtualHosts()).To(HaveLen(1))
		Expect(routeConfiguration.GetVirtualHosts()[0].Name).To(Equal("invalid_name"))
	})

	It("translates listener options", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)

		proxyClone.GetListeners()[0].Options = &v1.ListenerOptions{PerConnectionBufferLimitBytes: &wrappers.UInt32Value{Value: 4096}}

		snap, errs, report, err := translator.Translate(params, proxyClone)

		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		listeners := snap.GetResources(resource.ListenerTypeV3)
		Expect(listeners.Items).To(HaveKey("http-listener"))
		listenerResource := listeners.Items["http-listener"]
		listenerConfiguration := listenerResource.ResourceProto().(*envoy_config_listener_v3.Listener)
		Expect(listenerConfiguration).NotTo(BeNil())
		Expect(listenerConfiguration.PerConnectionBufferLimitBytes).To(MatchProto(&wrappers.UInt32Value{Value: 4096}))
	})

	Context("Auth configs", func() {
		It("will error if auth config is missing", func() {
			proxyClone := proto.Clone(proxy).(*v1.Proxy)
			proxyClone.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Options =
				&v1.VirtualHostOptions{
					Extauth: &extauth.ExtAuthExtension{
						Spec: &extauth.ExtAuthExtension_ConfigRef{
							ConfigRef: &core.ResourceRef{},
						},
					},
				}

			_, errs, _, err := translator.Translate(params, proxyClone)

			Expect(err).To(BeNil())
			Expect(errs.Validate()).To(HaveOccurred())
			Expect(errs.Validate().Error()).To(ContainSubstring("VirtualHost Error: ProcessingError. Reason: auth config not found:"))
		})
	})

	Context("service spec", func() {
		It("changes in service spec should create a different snapshot", func() {
			translate()
			oldVersion := snapshot.GetResources(resource.ClusterTypeV3).Version

			svcSpec := &v1plugins.ServiceSpec{
				PluginType: &v1plugins.ServiceSpec_Grpc{
					Grpc: &v1grpc.ServiceSpec{},
				},
			}
			upstream.UpstreamType.(*v1.Upstream_Static).SetServiceSpec(svcSpec)
			translate()
			newVersion := snapshot.GetResources(resource.ClusterTypeV3).Version
			Expect(oldVersion).ToNot(Equal(newVersion))
		})
	})

	Context("route no path", func() {
		BeforeEach(func() {
			matcher.PathSpecifier = nil
			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name: "test",
				},
			}
		})
		It("should error when path math is missing", func() {
			_, errs, report, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())
			Expect(errs.Validate()).To(HaveOccurred())
			invalidMatcherName := fmt.Sprintf("%s-route-0", virtualHostName)
			Expect(errs.Validate().Error()).To(ContainSubstring(fmt.Sprintf("Route Error: InvalidMatcherError. Reason: no path specifier provided. Route Name: %s", invalidMatcherName)))
			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
				{
					Type:   validation.RouteReport_Error_InvalidMatcherError,
					Reason: fmt.Sprintf("no path specifier provided. Route Name: %s", invalidMatcherName),
				},
			}
			Expect(report).To(Equal(expectedReport))
		})
		It("should error when path math is missing even if we have grpc spec", func() {
			dest := routes[0].GetRouteAction().GetSingle()
			dest.DestinationSpec = &v1.DestinationSpec{
				DestinationType: &v1.DestinationSpec_Grpc{
					Grpc: &v1grpc.DestinationSpec{
						Package:  "glootest",
						Function: "TestMethod",
						Service:  "TestService",
					},
				},
			}
			_, errs, report, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())
			Expect(errs.Validate()).To(HaveOccurred())
			invalidMatcherName := fmt.Sprintf("%s-route-0", virtualHostName)
			processingErrorName := fmt.Sprintf("%s-route-0-%s-matcher-0", virtualHostName, routes[0].Name)
			Expect(errs.Validate().Error()).To(ContainSubstring(
				fmt.Sprintf(
					"Route Error: InvalidMatcherError. Reason: no path specifier provided. Route Name: %s; Route Error: ProcessingError. Reason: *grpc.plugin: missing path for grpc route. Route Name: %s",
					invalidMatcherName,
					processingErrorName,
				)))

			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
				{
					Type:   validation.RouteReport_Error_InvalidMatcherError,
					Reason: fmt.Sprintf("no path specifier provided. Route Name: %s", invalidMatcherName),
				},
				{
					Type:   validation.RouteReport_Error_ProcessingError,
					Reason: fmt.Sprintf("*grpc.plugin: missing path for grpc route. Route Name: %s", processingErrorName),
				},
			}
			Expect(report).To(Equal(expectedReport))
		})
	})

	Context("route path match - sensitive case", func() {
		It("should translate path matcher with sensitive case", func() {

			routes[0].Matchers = []*matchers.Matcher{
				{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo",
					},
					CaseSensitive: &wrappers.BoolValue{Value: true},
				},
			}

			translate()
			fooRoute := routeConfiguration.VirtualHosts[0].Routes[0]
			Expect(fooRoute.Match.GetPrefix()).To(Equal("/foo"))
			Expect(fooRoute.Match.CaseSensitive).To(Equal(&wrappers.BoolValue{Value: true}))
		})

		It("should translate path matcher with case insensitive", func() {

			routes[0].Matchers = []*matchers.Matcher{
				{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo",
					},
					CaseSensitive: &wrappers.BoolValue{Value: false},
				},
			}

			translate()
			fooRoute := routeConfiguration.VirtualHosts[0].Routes[0]
			Expect(fooRoute.Match.GetPrefix()).To(Equal("/foo"))
			Expect(fooRoute.Match.CaseSensitive).To(Equal(&wrappers.BoolValue{Value: false}))
		})
	})

	Context("route header match", func() {
		It("should translate header matcher with no value to a PresentMatch", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name: "test",
				},
			}
			translate()
			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			presentMatch := headerMatch.GetPresentMatch()
			Expect(presentMatch).To(BeTrue())
		})

		It("should translate header matcher with value to exact match", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			exactMatch := headerMatch.GetExactMatch()
			Expect(exactMatch).To(Equal("testvalue"))
		})

		It("should translate header matcher with regex becomes regex match", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
					Regex: true,
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			regex := headerMatch.GetSafeRegexMatch().GetRegex()
			Expect(regex).To(Equal("testvalue"))
		})

		It("should translate header matcher with regex becomes regex match, with non default program size", func() {

			settings := &v1.Settings{
				Gloo: &v1.GlooOptions{
					RegexMaxProgramSize: &wrappers.UInt32Value{Value: 200},
				},
			}
			params.Ctx = settingsutil.WithSettings(params.Ctx, settings)

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
					Regex: true,
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.Name).To(Equal("test"))
			Expect(headerMatch.InvertMatch).To(Equal(false))
			regex := headerMatch.GetSafeRegexMatch().GetRegex()
			Expect(regex).To(Equal("testvalue"))
			maxsize := headerMatch.GetSafeRegexMatch().GetGoogleRe2().GetMaxProgramSize().GetValue()
			Expect(maxsize).To(BeEquivalentTo(200))
		})

		It("should translate header matcher logic inversion flag", func() {

			matcher.Headers = []*matchers.HeaderMatcher{
				{
					Name:        "test",
					InvertMatch: true,
				},
			}
			translate()

			headerMatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headerMatch.InvertMatch).To(Equal(true))
		})

		It("should default to '/' prefix matcher if none is provided", func() {
			matcher = nil
			translate()

			prefix := routeConfiguration.VirtualHosts[0].Routes[0].Match.GetPrefix()
			Expect(prefix).To(Equal("/"))
		})

		Context("Multiple matchers", func() {
			BeforeEach(func() {
				routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo",
						},
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/bar",
						},
					},
				}
			})

			It("should translate multiple matchers on a single Gloo route to multiple Envoy routes", func() {
				translate()
				fooRoute := routeConfiguration.VirtualHosts[0].Routes[0]
				barRoute := routeConfiguration.VirtualHosts[0].Routes[1]

				fooPrefix := fooRoute.Match.GetPrefix()
				barPrefix := barRoute.Match.GetPrefix()

				Expect(fooPrefix).To(Equal("/foo"))
				Expect(barPrefix).To(Equal("/bar"))

				Expect(fooRoute.Name).To(MatchRegexp("testRouteName-[0-9]*"))
				Expect(barRoute.Name).To(MatchRegexp("testRouteName-[0-9]*"))

				// the routes should be otherwise identical. wipe the matchers and names and compare them
				fooRoute.Match = &envoy_config_route_v3.RouteMatch{}
				barRoute.Match = &envoy_config_route_v3.RouteMatch{}
				fooRoute.Name = ""
				barRoute.Name = ""

				Expect(fooRoute).To(Equal(barRoute))
			})
		})
	})

	Context("non route_routeaction routes", func() {
		BeforeEach(func() {
			redirectRoute := &v1.Route{
				Action: &v1.Route_RedirectAction{
					RedirectAction: &v1.RedirectAction{
						ResponseCode: 400,
					},
				},
			}
			directResponseRoute := &v1.Route{
				Action: &v1.Route_DirectResponseAction{
					DirectResponseAction: &v1.DirectResponseAction{
						Status: 400,
					},
				},
			}
			routes = []*v1.Route{redirectRoute, directResponseRoute}
		})

		It("reports no errors with a redirect route or direct response route", func() {
			translate()
		})
	})

	Context("Health check config", func() {

		It("will error if required field is nil", func() {
			upstream.HealthChecks = []*gloo_envoy_core.HealthCheck{
				{
					Interval: DefaultHealthCheckInterval,
				},
			}
			report := translateWithError()

			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("will error if no health checker is supplied", func() {
			upstream.HealthChecks = []*gloo_envoy_core.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
				},
			}
			report := translateWithError()
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("can translate the http health check", func() {
			expectedResult := []*envoy_config_core_v3.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
					HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
							Host: "host",
							Path: "path",
							ServiceNameMatcher: &envoy_type_matcher_v3.StringMatcher{
								MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
									Prefix: "svc",
								},
							},
							RequestHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{},
							RequestHeadersToRemove: []string{},
							CodecClientType:        envoy_type_v3.CodecClientType_HTTP2,
							ExpectedStatuses:       []*envoy_type_v3.Int64Range{},
						},
					},
				},
			}
			var err error
			upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			translate()
			var msgList []proto.Message
			for _, v := range expectedResult {
				msgList = append(msgList, v)
			}
			Expect(cluster.HealthChecks).To(ConsistOfProtos(msgList...))
		})

		It("can translate the grpc health check", func() {
			expectedResult := []*envoy_config_core_v3.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
					HealthChecker: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck_{
						GrpcHealthCheck: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck{
							ServiceName: "svc",
							Authority:   "authority",
						},
					},
				},
			}
			var err error
			upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			translate()
			var msgList []proto.Message
			for _, v := range expectedResult {
				msgList = append(msgList, v)
			}
			Expect(cluster.HealthChecks).To(ConsistOfProtos(msgList...))
		})

		It("can properly translate outlier detection config", func() {
			dur := &duration.Duration{Seconds: 1}
			expectedResult := &envoy_config_cluster_v3.OutlierDetection{
				Consecutive_5Xx:                        DefaultThreshold,
				Interval:                               dur,
				BaseEjectionTime:                       dur,
				MaxEjectionPercent:                     DefaultThreshold,
				EnforcingConsecutive_5Xx:               DefaultThreshold,
				EnforcingSuccessRate:                   DefaultThreshold,
				SuccessRateMinimumHosts:                DefaultThreshold,
				SuccessRateRequestVolume:               DefaultThreshold,
				SuccessRateStdevFactor:                 nil,
				ConsecutiveGatewayFailure:              DefaultThreshold,
				EnforcingConsecutiveGatewayFailure:     nil,
				SplitExternalLocalOriginErrors:         true,
				ConsecutiveLocalOriginFailure:          nil,
				EnforcingConsecutiveLocalOriginFailure: nil,
				EnforcingLocalOriginSuccessRate:        nil,
			}
			upstream.OutlierDetection = api_conversion.ToGlooOutlierDetection(expectedResult)
			translate()
			Expect(cluster.OutlierDetection).To(MatchProto(expectedResult))
		})

		It("can properly validate outlier detection config", func() {
			expectedResult := &envoy_config_cluster_v3.OutlierDetection{}
			upstream.OutlierDetection = api_conversion.ToGlooOutlierDetection(expectedResult)
			report := translateWithError()
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("can translate health check with secret header", func() {
			params.Snapshot.Secrets = v1.SecretList{
				{
					Kind: &v1.Secret_Header{
						Header: &v1.HeaderSecret{
							Headers: map[string]string{
								"Authorization": "basic dXNlcjpwYXNzd29yZA==",
							},
						},
					},
					Metadata: &core.Metadata{
						Name:      "foo",
						Namespace: "bar",
					},
				},
			}

			expectedResult := []*envoy_config_core_v3.HealthCheck{
				{
					Timeout:            DefaultHealthCheckTimeout,
					Interval:           DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
					HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
							Host: "host",
							Path: "path",
							ServiceNameMatcher: &envoy_type_matcher_v3.StringMatcher{
								MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
									Prefix: "svc",
								},
							},
							RequestHeadersToAdd:    []*envoy_config_core_v3.HeaderValueOption{},
							RequestHeadersToRemove: []string{},
							CodecClientType:        envoy_type_v3.CodecClientType_HTTP2,
							ExpectedStatuses:       []*envoy_type_v3.Int64Range{},
						},
					},
				},
			}

			var err error
			upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())

			expectedResult[0].GetHttpHealthCheck().RequestHeadersToAdd = []*envoy_config_core_v3.HeaderValueOption{
				{
					Header: &envoy_config_core_v3.HeaderValue{
						Key:   "Authorization",
						Value: "basic dXNlcjpwYXNzd29yZA==",
					},
					Append: &wrappers.BoolValue{
						Value: true,
					},
				},
			}

			upstream.GetHealthChecks()[0].GetHttpHealthCheck().RequestHeadersToAdd = []*envoycore_sk.HeaderValueOption{
				{
					HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{
						HeaderSecretRef: &core.ResourceRef{
							Name:      "foo",
							Namespace: "bar",
						},
					},
					Append: &wrappers.BoolValue{
						Value: true,
					},
				},
			}

			snap, errs, report, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())
			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(snap).NotTo(BeNil())
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))

			clusters := snap.GetResources(resource.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())
			var msgList []proto.Message
			for _, v := range expectedResult {
				msgList = append(msgList, v)
			}
			Expect(cluster.HealthChecks).To(ConsistOfProtos(msgList...))
		})
	})

	Context("circuit breakers", func() {

		It("should NOT translate circuit breakers on upstream", func() {
			translate()
			Expect(cluster.CircuitBreakers).To(BeNil())
		})

		It("should translate circuit breakers on upstream", func() {

			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 1},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
				MaxRequests:        &wrappers.UInt32Value{Value: 3},
				MaxRetries:         &wrappers.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoy_config_cluster_v3.CircuitBreakers{
				Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(MatchProto(expectedCircuitBreakers))
		})

		It("should translate circuit breakers on settings", func() {

			settings.Gloo = &v1.GlooOptions{}
			settings.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 1},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
				MaxRequests:        &wrappers.UInt32Value{Value: 3},
				MaxRetries:         &wrappers.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoy_config_cluster_v3.CircuitBreakers{
				Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(MatchProto(expectedCircuitBreakers))
		})

		It("should override circuit breakers on upstream", func() {

			settings.Gloo = &v1.GlooOptions{}
			settings.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 11},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 12},
				MaxRequests:        &wrappers.UInt32Value{Value: 13},
				MaxRetries:         &wrappers.UInt32Value{Value: 14},
			}

			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &wrappers.UInt32Value{Value: 1},
				MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
				MaxRequests:        &wrappers.UInt32Value{Value: 3},
				MaxRetries:         &wrappers.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoy_config_cluster_v3.CircuitBreakers{
				Thresholds: []*envoy_config_cluster_v3.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(MatchProto(expectedCircuitBreakers))
		})
	})

	Context("eds", func() {

		It("should translate eds differently with different clusters", func() {
			translate()
			version1 := endpoints.Version
			// change the cluster
			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxRetries: &wrappers.UInt32Value{Value: 5},
			}
			translate()
			version2 := endpoints.Version
			Expect(version2).ToNot(Equal(version1))
		})
	})

	Context("lds", func() {

		var (
			localUpstream1 *v1.Upstream
			localUpstream2 *v1.Upstream
		)

		BeforeEach(func() {

			Expect(params.Snapshot.Upstreams).To(HaveLen(1))

			buildLocalUpstream := func(descriptors string) *v1.Upstream {
				return &v1.Upstream{
					Metadata: &core.Metadata{
						Name:      "test2",
						Namespace: "gloo-system",
					},
					UpstreamType: &v1.Upstream_Static{
						Static: &v1static.UpstreamSpec{
							ServiceSpec: &v1plugins.ServiceSpec{
								PluginType: &v1plugins.ServiceSpec_Grpc{
									Grpc: &v1grpc.ServiceSpec{
										GrpcServices: []*v1grpc.ServiceSpec_GrpcService{{
											PackageName: "foo",
											ServiceName: "bar",
										}},
										Descriptors: []byte(descriptors),
									},
								},
							},
							Hosts: []*v1static.Host{
								{
									Addr: "Test2",
									Port: 124,
								},
							},
						},
					},
				}
			}

			localUpstream1 = buildLocalUpstream("")
			localUpstream2 = buildLocalUpstream("")

		})

		It("should have same version and http filters when http filters with the same name are added in a different order", func() {
			translate()

			By("get the original version and http filters")

			// get version
			originalVersion := snapshot.GetResources(resource.ListenerTypeV3).Version

			// get http filters
			hcmFilter := listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err := glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			originalHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters

			By("add the upstreams and compare the new version and http filters")

			// add upstreams with same name
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream1)
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream2)
			Expect(params.Snapshot.Upstreams).To(HaveLen(3))

			translate()

			// get and compare version
			upstreamsVersion := snapshot.GetResources(resource.ListenerTypeV3).Version
			Expect(upstreamsVersion).ToNot(Equal(originalVersion))

			// get and compare http filters
			hcmFilter = listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err = glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			upstreamsHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters
			Expect(upstreamsHttpFilters).ToNot(Equal(originalHttpFilters))

			// reset modified global variables
			beforeEach()

			By("add the upstreams in the opposite order and compare the version and http filters")

			// add upstreams in the opposite order
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream2)
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream1)
			Expect(params.Snapshot.Upstreams).To(HaveLen(3))

			translate()

			// get and compare version
			flipOrderVersion := snapshot.GetResources(resource.ListenerTypeV3).Version
			Expect(flipOrderVersion).To(Equal(upstreamsVersion))

			// get and compare http filters
			hcmFilter = listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err = glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			flipOrderHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters
			Expect(flipOrderHttpFilters).To(Equal(upstreamsHttpFilters))
		})

	})

	Context("when handling cluster_header HTTP header name", func() {
		Context("with valid http header", func() {
			BeforeEach(func() {
				routes = []*v1.Route{{
					Name:     "testRouteClusterHeader",
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_ClusterHeader{
								ClusterHeader: "test-cluster",
							},
						},
					},
				}}
			})

			It("should translate valid HTTP header name", func() {
				translate()
				route := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute()
				Expect(route).ToNot(BeNil())
				cluster := route.GetClusterHeader()
				Expect(cluster).ToNot(BeNil())
				Expect(cluster).To(Equal("test-cluster"))
			})
		})

		Context("with invalid http header", func() {
			BeforeEach(func() {
				routes = []*v1.Route{{
					Name:     "testRouteClusterHeader",
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_ClusterHeader{
								ClusterHeader: "invalid:-cluster",
							},
						},
					},
				}}
			})

			It("should warn about invalid http header name", func() {
				_, _, report, _ := translator.Translate(params, proxy)
				routeReportWarning := report.GetListenerReports()[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0].GetWarnings()[0]
				reason := routeReportWarning.GetReason()
				Expect(reason).To(Equal("invalid:-cluster is an invalid HTTP header name"))
			})
		})

	})

	Context("when handling upstream groups", func() {

		var (
			upstream2     *v1.Upstream
			upstreamGroup *v1.UpstreamGroup
		)

		BeforeEach(func() {
			upstream2 = &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      "test2",
					Namespace: "gloo-system",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "Test2",
								Port: 124,
							},
						},
					},
				},
			}
			upstreamGroup = &v1.UpstreamGroup{
				Metadata: &core.Metadata{
					Name:      "test",
					Namespace: "gloo-system",
				},
				Destinations: []*v1.WeightedDestination{
					{
						Weight: 1,
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: upstream.Metadata.Ref(),
							},
						},
					},
					{
						Weight: 1,
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: upstream2.Metadata.Ref(),
							},
						},
					},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, upstream2)
			params.Snapshot.UpstreamGroups = v1.UpstreamGroupList{
				upstreamGroup,
			}
			ref := upstreamGroup.Metadata.Ref()
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_UpstreamGroup{
							UpstreamGroup: ref,
						},
					},
				},
			}}
		})

		It("should translate upstream groups", func() {
			translate()

			route := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute()
			Expect(route).ToNot(BeNil())
			clusters := route.GetWeightedClusters()
			Expect(clusters).ToNot(BeNil())
			Expect(clusters.TotalWeight.Value).To(BeEquivalentTo(2))
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
			Expect(clusters.Clusters[1].Name).To(Equal(UpstreamToClusterName(upstream2.Metadata.Ref())))
		})

		It("should error on invalid ref in upstream groups", func() {
			upstreamGroup.Destinations[0].Destination.GetUpstream().Name = "notexist"

			_, errs, report, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())
			err = errs.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("destination # 1: upstream not found: list did not find upstream gloo-system.notexist"))

			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Warnings = []*validation.RouteReport_Warning{
				{
					Type:   validation.RouteReport_Warning_InvalidDestinationWarning,
					Reason: "invalid destination in weighted destination list: *v1.Upstream { gloo-system.notexist } not found",
				},
			}
			Expect(report).To(Equal(expectedReport))
		})

		It("should use upstreamGroup's namespace as default if namespace is omitted on upstream destination", func() {
			upstreamGroup.Destinations[0].Destination.GetUpstream().Namespace = ""

			translate()

			clusters := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters()
			Expect(clusters).ToNot(BeNil())
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
			Expect(clusters.Clusters[1].Name).To(Equal(UpstreamToClusterName(upstream2.Metadata.Ref())))
		})
	})

	Context("when handling missing upstream groups", func() {
		BeforeEach(func() {
			metadata := core.Metadata{
				Name:      "missing",
				Namespace: "gloo-system",
			}
			ref := metadata.Ref()

			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_UpstreamGroup{
							UpstreamGroup: ref,
						},
					},
				},
			}}
		})

		It("should set a ClusterSpecifier on the referring route", func() {
			snap, _, _, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())

			routes := snap.GetResources(resource.RouteTypeV3)
			routesProto := routes.Items["http-listener-routes"]

			routeConfig := routesProto.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			clusterSpecifier := routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetClusterSpecifier()
			clusterRouteAction := clusterSpecifier.(*envoy_config_route_v3.RouteAction_Cluster)
			Expect(clusterRouteAction.Cluster).To(Equal(""))
		})
	})

	Context("when handling endpoints", func() {
		var (
			claConfiguration *envoy_config_endpoint_v3.ClusterLoadAssignment
			annotations      map[string]string
		)
		BeforeEach(func() {
			claConfiguration = nil
			annotations = map[string]string{"testkey": "testvalue"}

			upstream.UpstreamType = &v1.Upstream_Kube{
				Kube: &v1kubernetes.UpstreamSpec{},
			}
			ref := upstream.Metadata.Ref()
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: &core.Metadata{
						Name:        "test",
						Namespace:   "gloo-system",
						Annotations: annotations,
					},
					Upstreams: []*core.ResourceRef{
						ref,
					},
					Address: "1.2.3.4",
					Port:    1234,
				},
			}
		})
		It("should transfer annotations to snapshot", func() {
			translate()

			endpoints := snapshot.GetResources(resource.EndpointTypeV3)

			clusterName := getEndpointClusterName(upstream)

			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment)
			Expect(claConfiguration).NotTo(BeNil())
			Expect(claConfiguration.ClusterName).To(Equal(clusterName))
			Expect(claConfiguration.Endpoints).To(HaveLen(1))
			Expect(claConfiguration.Endpoints[0].LbEndpoints).To(HaveLen(len(params.Snapshot.Endpoints)))
			filterMetadata := claConfiguration.Endpoints[0].LbEndpoints[0].GetMetadata().GetFilterMetadata()

			Expect(filterMetadata).NotTo(BeNil())
			Expect(filterMetadata).To(HaveKey(SoloAnnotations))
			Expect(filterMetadata[SoloAnnotations].Fields).To(HaveKey("testkey"))
			Expect(filterMetadata[SoloAnnotations].Fields["testkey"].GetStringValue()).To(Equal("testvalue"))
		})
	})

	Context("when handling subsets", func() {
		var (
			claConfiguration *envoy_config_endpoint_v3.ClusterLoadAssignment
		)
		BeforeEach(func() {
			claConfiguration = nil

			upstream.UpstreamType = &v1.Upstream_Kube{
				Kube: &v1kubernetes.UpstreamSpec{
					SubsetSpec: &v1plugins.SubsetSpec{
						Selectors: []*v1plugins.Selector{{
							Keys: []string{
								"testkey",
							},
						}},
					},
				},
			}
			ref := upstream.Metadata.Ref()
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: &core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
						Labels:    map[string]string{"testkey": "testvalue"},
					},
					Upstreams: []*core.ResourceRef{
						ref,
					},
					Address: "1.2.3.4",
					Port:    1234,
				},
			}

			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: upstream.Metadata.Ref(),
								},
								Subset: &v1.Subset{
									Values: map[string]string{
										"testkey": "testvalue",
									},
								},
							},
						},
					},
				},
			}}

		})

		translateWithEndpoints := func() {
			translate()

			endpoints := snapshot.GetResources(resource.EndpointTypeV3)
			clusterName := getEndpointClusterName(upstream)
			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment)
			Expect(claConfiguration).NotTo(BeNil())
			Expect(claConfiguration.ClusterName).To(Equal(clusterName))
			Expect(claConfiguration.Endpoints).To(HaveLen(1))
			Expect(claConfiguration.Endpoints[0].LbEndpoints).To(HaveLen(len(params.Snapshot.Endpoints)))
		}

		Context("when happy path", func() {

			It("should transfer labels to envoy", func() {
				translateWithEndpoints()

				endpointMeta := claConfiguration.Endpoints[0].LbEndpoints[0].Metadata
				fields := endpointMeta.FilterMetadata["envoy.lb"].Fields
				Expect(fields).To(HaveKey("testkey"))
				Expect(fields["testkey"]).To(MatchProto(sv("testvalue")))
			})

			It("should add subset to cluster", func() {
				translateWithEndpoints()

				Expect(cluster.LbSubsetConfig).ToNot(BeNil())
				Expect(cluster.LbSubsetConfig.FallbackPolicy).To(Equal(envoy_config_cluster_v3.Cluster_LbSubsetConfig_ANY_ENDPOINT))
				Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].Keys).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].Keys[0]).To(Equal("testkey"))
			})
			It("should add subset to route", func() {
				translateWithEndpoints()

				metadataMatch := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute().GetMetadataMatch()
				fields := metadataMatch.FilterMetadata["envoy.lb"].Fields
				Expect(fields).To(HaveKeyWithValue("testkey", sv("testvalue")))
			})
		})

		It("should create empty value if missing labels on the endpoint are provided in the upstream", func() {
			params.Snapshot.Endpoints[0].Metadata.Labels = nil
			translateWithEndpoints()
			endpointMeta := claConfiguration.Endpoints[0].LbEndpoints[0].Metadata
			Expect(endpointMeta).ToNot(BeNil())
			Expect(endpointMeta.FilterMetadata).To(HaveKey("envoy.lb"))
			fields := endpointMeta.FilterMetadata["envoy.lb"].Fields
			Expect(fields).To(HaveKey("testkey"))
			Expect(fields["testkey"]).To(MatchProto(sv("")))
		})

		Context("subset in route doesnt match subset in upstream", func() {

			BeforeEach(func() {
				routes = []*v1.Route{{
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Upstream{
										Upstream: (upstream.Metadata.Ref()),
									},
									Subset: &v1.Subset{
										Values: map[string]string{
											"nottestkey": "value",
										},
									},
								},
							},
						},
					},
				}}
			})

			It("should error the route", func() {
				_, errs, report, err := translator.Translate(params, proxy)
				Expect(err).NotTo(HaveOccurred())
				Expect(errs.Validate()).To(HaveOccurred())
				Expect(errs.Validate().Error()).To(ContainSubstring("route has a subset config, but none of the subsets in the upstream match it"))
				processingErrorName := fmt.Sprintf("%s-route-0-matcher-0", virtualHostName)
				expectedReport := validationutils.MakeReport(proxy)
				expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
					{
						Type:   validation.RouteReport_Error_ProcessingError,
						Reason: fmt.Sprintf("route has a subset config, but none of the subsets in the upstream match it. Route Name: %s", processingErrorName),
					},
				}
				Expect(report).To(Equal(expectedReport))
			})
		})

		Context("when a route table is missing", func() {

			BeforeEach(func() {
				routes = []*v1.Route{{
					Matchers: []*matchers.Matcher{matcher},
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Upstream{
										Upstream: &core.ResourceRef{Name: "do", Namespace: "notexist"},
									},
								},
							},
						},
					},
				}}
			})

			It("should warn a route when a destination is missing", func() {
				_, errs, report, err := translator.Translate(params, proxy)
				Expect(err).NotTo(HaveOccurred())
				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(errs.ValidateStrict()).To(HaveOccurred())
				Expect(errs.ValidateStrict().Error()).To(ContainSubstring("*v1.Upstream { notexist.do } not found"))
				expectedReport := validationutils.MakeReport(proxy)
				expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Warnings = []*validation.RouteReport_Warning{
					{
						Type:   validation.RouteReport_Warning_InvalidDestinationWarning,
						Reason: "*v1.Upstream { notexist.do } not found",
					},
				}

				Expect(report).To(Equal(expectedReport))
			})
		})
	})

	Context("when translating a route that points directly to a service", func() {

		var fakeUsList v1.UpstreamList

		BeforeEach(func() {

			// The kube service that we want to route to
			svc := skkube.NewService("ns-1", "svc-1")
			svc.Spec = k8scorev1.ServiceSpec{
				Ports: []k8scorev1.ServicePort{
					{
						Name:       "port-1",
						Port:       8080,
						TargetPort: intstr.FromInt(80),
					},
					{
						Name:       "port-2",
						Port:       8081,
						TargetPort: intstr.FromInt(8081),
					},
				},
			}
			// These are the "fake" upstreams that represent the above service in the snapshot
			fakeUsList = kubernetes.KubeServicesToUpstreams(context.TODO(), skkube.ServiceList{svc})
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, fakeUsList...)

			// We need to manually add some fake endpoints for the above kubernetes services to the snapshot
			// Normally these would have been discovered by EDS
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: &core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.1", svc.Spec.Ports[0].Port),
					},
					Port:      uint32(svc.Spec.Ports[0].Port),
					Address:   "192.168.0.1",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: &core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.2", svc.Spec.Ports[1].Port),
					},
					Port:      uint32(svc.Spec.Ports[1].Port),
					Address:   "192.168.0.2",
					Upstreams: []*core.ResourceRef{(fakeUsList[1].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: &core.ResourceRef{
							Namespace: svc.Namespace,
							Name:      svc.Name,
						},
						Port: uint32(svc.Spec.Ports[0].Port),
					},
				},
			}
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &serviceDestination,
						},
					},
				},
			}}
		})

		It("generates the expected envoy route configuration", func() {
			translate()

			// Clusters have been created for the two "fake" upstreams
			clusters := snapshot.GetResources(resource.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())
			clusterResource = clusters.Items[UpstreamToClusterName(fakeUsList[1].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())

			// A route to the kube service has been configured
			routes := snapshot.GetResources(resource.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoy_config_route_v3.Route_Route)
			Expect(ok).To(BeTrue())
			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoy_config_route_v3.RouteAction_Cluster)
			Expect(ok).To(BeTrue())
			Expect(clusterAction.Cluster).To(Equal(UpstreamToClusterName(fakeUsList[0].Metadata.Ref())))
		})
	})

	Context("when translating a route that points to a Consul service", func() {

		var (
			fakeUsList v1.UpstreamList
			dc         = func(dataCenterName string) string {
				return constants.ConsulDataCenterKeyPrefix + dataCenterName
			}
			tag = func(tagName string) string {
				return constants.ConsulTagKeyPrefix + tagName
			}

			trueValue = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: constants.ConsulEndpointMetadataMatchTrue,
				},
			}
			falseValue = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: constants.ConsulEndpointMetadataMatchFalse,
				},
			}
		)

		const (
			svcName = "my-consul-svc"

			// Data centers
			east = "east"
			west = "west"

			// Tags
			dev  = "dev"
			prod = "prod"

			yes = constants.ConsulEndpointMetadataMatchTrue
			no  = constants.ConsulEndpointMetadataMatchFalse
		)

		BeforeEach(func() {

			// Metadata for the Consul service that we want to route to
			svc := &consul.ServiceMeta{
				Name:        svcName,
				DataCenters: []string{east, west},
				Tags:        []string{dev, prod},
			}
			// These are the "fake" upstreams that represent the above service in the snapshot
			initialUsList := consul.CreateUpstreamsFromService(svc, nil)
			if len(initialUsList) == 1 {
				fakeUsList = v1.UpstreamList{consul.CreateUpstreamsFromService(svc, nil)[0]}
			} else {
				fakeUsList = v1.UpstreamList{}
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, fakeUsList...)

			// We need to manually add some fake endpoints for the above Consul service
			// Normally these would have been discovered by EDS
			params.Snapshot.Endpoints = v1.EndpointList{
				// 2 prod endpoints, 1 in each data center, 1 dev endpoint in west data center
				{
					Metadata: &core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_1",
						Labels: map[string]string{
							dc(east):  yes,
							dc(west):  no,
							tag(dev):  no,
							tag(prod): yes,
						},
					},
					Port:      1001,
					Address:   "1.0.0.1",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: &core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_2",
						Labels: map[string]string{
							dc(east):  no,
							dc(west):  yes,
							tag(dev):  no,
							tag(prod): yes,
						},
					},
					Port:      2001,
					Address:   "2.0.0.1",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: &core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_3",
						Labels: map[string]string{
							dc(east):  no,
							dc(west):  yes,
							tag(dev):  yes,
							tag(prod): no,
						},
					},
					Port:      2002,
					Address:   "2.0.0.2",
					Upstreams: []*core.ResourceRef{(fakeUsList[0].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Consul{
					Consul: &v1.ConsulServiceDestination{
						ServiceName: svcName,
						Tags:        []string{prod},
						DataCenters: []string{east},
					},
				},
			}
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &serviceDestination,
						},
					},
				},
			}}
		})

		It("generates the expected envoy route configuration", func() {
			translate()

			// A cluster has been created for the "fake" upstream and has the expected subset config
			clusters := snapshot.GetResources(resource.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(3))
			// Order is important here
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(ConsistOfProtos(
				&envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west)},
				},
				&envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{tag(dev), tag(prod)},
				},
				&envoy_config_cluster_v3.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west), tag(dev), tag(prod)},
				},
			))

			// A route to the kube service has been configured
			routes := snapshot.GetResources(resource.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoy_config_route_v3.Route_Route)
			Expect(ok).To(BeTrue())

			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoy_config_route_v3.RouteAction_Cluster)
			Expect(ok).To(BeTrue())
			Expect(clusterAction.Cluster).To(Equal(UpstreamToClusterName(fakeUsList[0].Metadata.Ref())))

			Expect(routeAction.Route).NotTo(BeNil())
			Expect(routeAction.Route.MetadataMatch).NotTo(BeNil())
			metadata, ok := routeAction.Route.MetadataMatch.FilterMetadata[EnvoyLb]
			Expect(ok).To(BeTrue())
			Expect(metadata.Fields).To(HaveLen(4))
			Expect(metadata.Fields[dc(east)]).To(Equal(trueValue))
			Expect(metadata.Fields[dc(west)]).To(Equal(falseValue))
			Expect(metadata.Fields[tag(dev)]).To(Equal(falseValue))
			Expect(metadata.Fields[tag(prod)]).To(Equal(trueValue))
		})
	})

	Context("when translating a route that points to an AWS lambda", func() {

		createLambdaUpstream := func(namespace, name, region string, lambdaFuncs []*aws.LambdaFunctionSpec) *v1.Upstream {
			return &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      name,
					Namespace: namespace,
				},
				DiscoveryMetadata: nil,
				UpstreamType: &v1.Upstream_Aws{
					Aws: &aws.UpstreamSpec{
						SecretRef: &core.ResourceRef{
							Name:      "my-aws-secret",
							Namespace: "my-namespace",
						},
						Region:          region,
						LambdaFunctions: lambdaFuncs,
					},
				},
			}
		}

		BeforeEach(func() {
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams,
				createLambdaUpstream("my-namespace", "lambda-upstream-1", "us-east-1",
					[]*aws.LambdaFunctionSpec{
						{
							LogicalName: "usEast1Lambda1",
						},
						{
							LogicalName: "usEast1Lambda2",
						},
					}),
				createLambdaUpstream("my-namespace", "lambda-upstream-2", "us-east-2",
					[]*aws.LambdaFunctionSpec{
						{
							LogicalName: "usEast2Lambda1",
						},
						{
							LogicalName: "usEast2Lambda2",
						},
					}))

			secret := &v1.Secret{
				Metadata: &core.Metadata{
					Name:      "my-aws-secret",
					Namespace: "my-namespace",
				},
				Kind: &v1.Secret_Aws{
					Aws: &v1.AwsSecret{
						AccessKey: "a",
						SecretKey: "a",
					},
				},
			}

			params.Snapshot.Secrets = v1.SecretList{secret}
		})

		It("has no errors when pointing to a valid lambda", func() {
			validLambdaRoute := &v1.Route{Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "lambda-upstream-1",
									Namespace: "my-namespace",
								},
							},
							DestinationSpec: &v1.DestinationSpec{
								DestinationType: &v1.DestinationSpec_Aws{
									Aws: &aws.DestinationSpec{
										LogicalName: "usEast1Lambda1",
									},
								},
							},
						},
					},
				}}}

			routes := proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()
			proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Routes = append(routes, validLambdaRoute)

			translate()
		})

		It("reports error when pointing to a lambda function that doesn't exist", func() {
			invalidLambdaRoute := &v1.Route{Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "lambda-upstream-1",
									Namespace: "my-namespace",
								},
							},
							DestinationSpec: &v1.DestinationSpec{
								DestinationType: &v1.DestinationSpec_Aws{
									Aws: &aws.DestinationSpec{
										LogicalName: "nonexistentLambdaFunc",
									},
								},
							},
						},
					},
				}}}

			routes := proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()
			proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Routes = append(routes, invalidLambdaRoute)
			_, resourceReport, _, _ := translator.Translate(params, proxy)
			Expect(resourceReport.Validate()).To(HaveOccurred())
			Expect(resourceReport.Validate().Error()).To(ContainSubstring("a route references nonexistentLambdaFunc AWS lambda which does not exist on the route's upstream"))
		})

		It("reports error when route has Multi Cluster destination and points to at least one lambda function that doesn't exist", func() {
			invalidLambdaRoute := &v1.Route{Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Multi{
						Multi: &v1.MultiDestination{
							Destinations: []*v1.WeightedDestination{
								{
									Destination: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "aws-lambda-upstream",
												Namespace: "my-namespace",
											},
										},
										DestinationSpec: &v1.DestinationSpec{
											DestinationType: &v1.DestinationSpec_Aws{
												Aws: &aws.DestinationSpec{
													LogicalName: "nonexistentLambdaFunc",
												},
											},
										},
									},
								},
							},
						},
					},
				}}}

			routes := proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].GetRoutes()
			proxy.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Routes = append(routes, invalidLambdaRoute)
			_, resourceReport, _, _ := translator.Translate(params, proxy)
			Expect(resourceReport.Validate()).To(HaveOccurred())
			Expect(resourceReport.Validate().Error()).To(ContainSubstring("a route references nonexistentLambdaFunc AWS lambda which does not exist on the route's upstream"))
		})

	})

	Context("Route plugin", func() {
		var (
			routePlugin *routePluginMock
		)
		BeforeEach(func() {
			routePlugin = &routePluginMock{}
			registeredPlugins = append(registeredPlugins, routePlugin)
		})

		It("should have the virtual host when processing route", func() {
			hasVHost := false
			routePlugin.ProcessRouteFunc = func(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
				if params.VirtualHost != nil {
					if params.VirtualHost.GetName() == "virt1" {
						hasVHost = true
					}
				}
				return nil
			}

			translate()
			Expect(hasVHost).To(BeTrue())
		})

	})

	Context("EndpointPlugin", func() {
		var (
			endpointPlugin *endpointPluginMock
			upstreamList   v1.UpstreamList
		)
		BeforeEach(func() {
			endpointPlugin = &endpointPluginMock{}
			registeredPlugins = append(registeredPlugins, endpointPlugin)
			upstreamList = params.Snapshot.Upstreams.Clone()
		})

		AfterEach(func() {
			params.Snapshot.Upstreams = upstreamList
		})

		It("should call the endpoint plugin", func() {
			additionalEndpoint := &envoy_config_endpoint_v3.LocalityLbEndpoints{
				Locality: &envoy_config_core_v3.Locality{
					Region: "region",
					Zone:   "a",
				},
				Priority: 10,
			}

			endpointPlugin.ProcessEndpointFunc = func(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
				Expect(out.GetEndpoints()).To(HaveLen(1))
				Expect(out.GetClusterName()).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
				Expect(out.GetEndpoints()[0].GetLbEndpoints()).To(HaveLen(1))

				out.Endpoints = append(out.Endpoints, additionalEndpoint)
				return nil
			}

			translate()
			endpointResource := endpoints.Items["test_gloo-system"]
			endpoint := endpointResource.ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment)
			Expect(endpoint).NotTo(BeNil())
			Expect(endpoint.Endpoints).To(HaveLen(2))
			Expect(endpoint.Endpoints[1]).To(MatchProto(additionalEndpoint))
		})

		It("should call the endpoint plugin with an empty endpoint", func() {
			// Create an empty consul upstream just to get EDS
			emptyUpstream := &v1.Upstream{
				Metadata: &core.Metadata{
					Namespace: "empty_namespace",
					Name:      "empty_name",
				},
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul2.UpstreamSpec{},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, emptyUpstream)

			foundEmptyUpstream := false

			endpointPlugin.ProcessEndpointFunc = func(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
				if in.Metadata.Name == emptyUpstream.Metadata.Name &&
					in.Metadata.Namespace == emptyUpstream.Metadata.Namespace {
					foundEmptyUpstream = true
				}
				return nil
			}

			translate()
			Expect(foundEmptyUpstream).To(BeTrue())
		})

	})

	Context("Route option on direct response actions", func() {

		BeforeEach(func() {
			routes = []*v1.Route{{
				Matchers: []*matchers.Matcher{matcher},
				Action: &v1.Route_DirectResponseAction{
					DirectResponseAction: &v1.DirectResponseAction{
						Status: 405,
						Body:   "Unsupported HTTP method",
					},
				},
				Options: &v1.RouteOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						ResponseHeadersToAdd: []*headers.HeaderValueOption{{
							Header: &headers.HeaderValue{
								Key:   "client-id",
								Value: "%REQ(client-id)%",
							},
							Append: &wrappers.BoolValue{
								Value: false,
							},
						}},
					},
				},
			}}
		})

		It("should have the virtual host when processing route", func() {
			translate()

			// A route to the kube service has been configured
			routes := snapshot.GetResources(resource.RouteTypeV3)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoy_config_route_v3.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))

			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Routes[0].ResponseHeadersToAdd).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Routes[0].ResponseHeadersToAdd).To(ConsistOf(
				&envoy_config_core_v3.HeaderValueOption{
					Header: &envoy_config_core_v3.HeaderValue{
						Key:   "client-id",
						Value: "%REQ(client-id)%",
					},
					Append: &wrappers.BoolValue{
						Value: false,
					},
				},
			))
		})

	})

	Context("TCP", func() {
		It("can properly create a tcp listener", func() {
			translate()
			listeners := snapshot.GetResources(resource.ListenerTypeV3).Items
			Expect(listeners).NotTo(HaveLen(0))
			val, found := listeners["tcp-listener"]
			Expect(found).To(BeTrue())
			listener, ok := val.ResourceProto().(*envoy_config_listener_v3.Listener)
			Expect(ok).To(BeTrue())
			Expect(listener.GetName()).To(Equal("tcp-listener"))
			Expect(listener.GetFilterChains()).To(HaveLen(1))
			fc := listener.GetFilterChains()[0]
			Expect(fc.Filters).To(HaveLen(1))
			tcpFilter := fc.Filters[0]
			cfg := tcpFilter.GetTypedConfig()
			Expect(cfg).NotTo(BeNil())
			var typedCfg envoytcp.TcpProxy
			Expect(ParseTypedConfig(tcpFilter, &typedCfg)).NotTo(HaveOccurred())
			clusterSpec := typedCfg.GetCluster()
			Expect(clusterSpec).To(Equal("test_gloo-system"))
			Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
		})
	})

	Context("Ssl - cluster", func() {

		var (
			tlsConf *v1.TlsSecret
		)
		BeforeEach(func() {

			tlsConf = &v1.TlsSecret{}
			secret := &v1.Secret{
				Metadata: &core.Metadata{
					Name:      "name",
					Namespace: "namespace",
				},
				Kind: &v1.Secret_Tls{
					Tls: tlsConf,
				},
			}
			ref := secret.Metadata.Ref()
			upstream.SslConfig = &v1.UpstreamSslConfig{
				SslSecrets: &v1.UpstreamSslConfig_SecretRef{
					SecretRef: ref,
				},
			}
			params = plugins.Params{
				Ctx: context.Background(),
				Snapshot: &v1.ApiSnapshot{
					Secrets:   v1.SecretList{secret},
					Upstreams: v1.UpstreamList{upstream},
				},
			}

		})

		tlsContext := func() *envoyauth.UpstreamTlsContext {
			clusters := snapshot.GetResources(resource.ClusterTypeV3)
			clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
			cluster := clusterResource.ResourceProto().(*envoy_config_cluster_v3.Cluster)

			return glooutils.MustAnyToMessage(cluster.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
		}

		It("should process an upstream with tls config", func() {
			translate()
			Expect(tlsContext()).ToNot(BeNil())
		})

		It("should process an upstream with tls config", func() {

			tlsConf.PrivateKey = "private"
			tlsConf.CertChain = "certchain"

			translate()
			Expect(tlsContext()).ToNot(BeNil())
			Expect(tlsContext().CommonTlsContext.TlsCertificates[0].PrivateKey.GetInlineString()).To(Equal("private"))
			Expect(tlsContext().CommonTlsContext.TlsCertificates[0].CertificateChain.GetInlineString()).To(Equal("certchain"))
		})

		It("should process an upstream with rootca", func() {
			tlsConf.RootCa = "rootca"

			translate()
			Expect(tlsContext()).ToNot(BeNil())
			Expect(tlsContext().CommonTlsContext.GetValidationContext().TrustedCa.GetInlineString()).To(Equal("rootca"))
		})

		Context("SslParameters", func() {

			It("should set upstream SslParameters if defined on upstream", func() {
				upstreamSslParameters := &v1.SslParameters{
					CipherSuites: []string{"AES256-SHA", "AES256-GCM-SHA384"},
				}

				settingsSslParameters := &v1.SslParameters{
					CipherSuites: []string{"ECDHE-RSA-AES128-SHA"},
				}

				upstream.SslConfig.Parameters = upstreamSslParameters
				settings.UpstreamOptions = &v1.UpstreamOptions{
					SslParameters: settingsSslParameters,
				}

				translate()
				Expect(tlsContext()).ToNot(BeNil())
				Expect(tlsContext().CommonTlsContext.TlsParams.CipherSuites).To(Equal(upstreamSslParameters.CipherSuites))
			})

			It("should set settings.UpstreamOptions SslParameters if none defined on upstream", func() {
				settingsSslParameters := &v1.SslParameters{
					CipherSuites: []string{"ECDHE-RSA-AES128-SHA"},
				}

				upstream.SslConfig.Parameters = nil
				settings.UpstreamOptions = &v1.UpstreamOptions{
					SslParameters: settingsSslParameters,
				}

				translate()
				Expect(tlsContext()).ToNot(BeNil())
				Expect(tlsContext().CommonTlsContext.TlsParams.CipherSuites).To(Equal(settingsSslParameters.CipherSuites))
			})

		})

		Context("failure", func() {

			It("should fail with only private key", func() {

				tlsConf.PrivateKey = "private"
				_, errs, _, err := translator.Translate(params, proxy)

				Expect(err).To(BeNil())
				Expect(errs.Validate()).To(HaveOccurred())
				Expect(errs.Validate().Error()).To(ContainSubstring("both or none of cert chain and private key must be provided"))
			})
			It("should fail with only cert chain", func() {

				tlsConf.CertChain = "certchain"

				_, errs, _, err := translator.Translate(params, proxy)

				Expect(err).To(BeNil())
				Expect(errs.Validate()).To(HaveOccurred())
				Expect(errs.Validate().Error()).To(ContainSubstring("both or none of cert chain and private key must be provided"))
			})
		})
	})

	Context("Ssl", func() {

		var (
			listener *envoy_config_listener_v3.Listener
		)

		prepSsl := func(s []*v1.SslConfig) {
			httpListener := &v1.Listener{
				Name:        "http-listener",
				BindAddress: "127.0.0.1",
				BindPort:    80,
				ListenerType: &v1.Listener_HttpListener{
					HttpListener: &v1.HttpListener{
						VirtualHosts: []*v1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes:  routes,
						}},
					},
				},
				SslConfigurations: s,
			}
			proxy.Listeners = []*v1.Listener{
				httpListener,
			}
		}

		prep := func(s []*v1.SslConfig) {
			prepSsl(s)
			translate()

			listeners := snapshot.GetResources(resource.ListenerTypeV3).Items
			Expect(listeners).To(HaveLen(1))
			val, found := listeners["http-listener"]
			Expect(found).To(BeTrue())
			listener = val.ResourceProto().(*envoy_config_listener_v3.Listener)
		}
		tlsContext := func(fc *envoy_config_listener_v3.FilterChain) *envoyauth.DownstreamTlsContext {
			if fc.TransportSocket == nil {
				return nil
			}
			return glooutils.MustAnyToMessage(fc.TransportSocket.GetTypedConfig()).(*envoyauth.DownstreamTlsContext)
		}
		Context("files", func() {

			It("should translate ssl correctly", func() {
				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
					},
				})
				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())

				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})

			It("should not merge 2 ssl config if they are different", func() {
				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert1",
								TlsKey:  "key1",
							},
						},
						SniDomains: []string{
							"sni1",
						},
					},
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert2",
								TlsKey:  "key2",
							},
						},
						SniDomains: []string{
							"sni2",
						},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(2))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})

			It("should merge 2 ssl config if they are the same", func() {
				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
					},
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})

			It("should reject configs if different FilterChains have identical FilterChainMatches", func() {
				filterChains := []*envoy_config_listener_v3.FilterChain{
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							DestinationPort: &wrappers.UInt32Value{Value: 1},
						},
					},
					{
						FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
							DestinationPort: &wrappers.UInt32Value{Value: 1},
						},
					},
				}
				report := &validation.ListenerReport{}
				CheckForDuplicateFilterChainMatches(filterChains, report)
				Expect(report.Errors).NotTo(BeNil())
				Expect(report.Errors).To(HaveLen(1))
				Expect(report.Errors[0].Type).To(Equal(validation.ListenerReport_Error_SSLConfigError))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should combine sni matches", func() {
				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
						SniDomains: []string{"b.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetFilename()).To(Equal("cert"))
				Expect(cert.GetPrivateKey().GetFilename()).To(Equal("key"))
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com", "b.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should combine 1 that has and 1 that doesn't have sni", func() {

				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
					},
					{
						SslSecrets: &v1.SslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
							},
						},
						SniDomains: []string{"b.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(BeEmpty())
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
		})
		Context("secret refs", func() {
			It("should combine sni matches ", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain",
							PrivateKey: "key",
						},
					},
				})

				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"b.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(1))
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key"))
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com", "b.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should not combine when not matching", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain1",
							PrivateKey: "key1",
						},
					},
				}, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo2",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain2",
							PrivateKey: "key2",
							RootCa:     "rootca2",
						},
					},
				}, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo", // check same name with different ns
						Namespace: "solo.io2",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain3",
							PrivateKey: "key3",
						},
					},
				})

				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo2",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"b.com"},
					},
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io2",
							},
						},
						SniDomains: []string{"c.com"},
					},
					{
						Parameters: &v1.SslParameters{
							MinimumProtocolVersion: v1.SslParameters_TLSv1_2,
						},
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io2",
							},
						},
						SniDomains: []string{"d.com"},
					},
					{
						Parameters: &v1.SslParameters{
							MinimumProtocolVersion: v1.SslParameters_TLSv1_2,
						},
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io2",
							},
						},
						SniDomains: []string{"d.com", "e.com"},
					},
				})

				Expect(listener.GetFilterChains()).To(HaveLen(4))
				By("checking first filter chain")
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain1"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key1"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com"}))

				By("checking second filter chain")
				fc = listener.GetFilterChains()[1]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain2"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key2"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext().GetTrustedCa().GetInlineString()).To(Equal("rootca2"))
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"b.com"}))

				By("checking third filter chain")
				fc = listener.GetFilterChains()[2]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain3"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key3"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"c.com"}))

				By("checking forth filter chain")
				fc = listener.GetFilterChains()[3]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain3"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key3"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"d.com", "e.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
			It("should error when different parameters have the same sni domains", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain1",
							PrivateKey: "key1",
						},
					},
				})

				prepSsl([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						Parameters: &v1.SslParameters{
							MinimumProtocolVersion: v1.SslParameters_TLSv1_2,
						},
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
				})
				_, errs, _, _ := translator.Translate(params, proxy)
				proxyKind := resources.Kind(proxy)
				_, reports := errs.Find(proxyKind, proxy.Metadata.Ref())
				Expect(reports.Errors.Error()).To(ContainSubstring("Tried to apply multiple filter chains with the" +
					" same FilterChainMatch {server_names:\"a.com\"}. This is usually caused by overlapping sniDomains or multiple empty sniDomains in virtual services"))
			})
			It("should error when different parameters have no sni domains", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain1",
							PrivateKey: "key1",
						},
					},
				})

				prepSsl([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
					},
					{
						Parameters: &v1.SslParameters{
							MinimumProtocolVersion: v1.SslParameters_TLSv1_2,
						},
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
					},
				})
				_, errs, _, _ := translator.Translate(params, proxy)
				proxyKind := resources.Kind(proxy)
				_, reports := errs.Find(proxyKind, proxy.Metadata.Ref())
				Expect(reports.Errors.Error()).To(ContainSubstring("Tried to apply multiple filter chains with the" +
					" same FilterChainMatch {}. This is usually caused by overlapping sniDomains or multiple empty sniDomains in virtual services"))
			})
			It("should work when different parameters have different sni domains", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: &core.Metadata{
						Name:      "solo",
						Namespace: "solo.io",
					},
					Kind: &v1.Secret_Tls{
						Tls: &v1.TlsSecret{
							CertChain:  "chain1",
							PrivateKey: "key1",
						},
					},
				})

				prep([]*v1.SslConfig{
					{
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"a.com"},
					},
					{
						Parameters: &v1.SslParameters{
							MinimumProtocolVersion: v1.SslParameters_TLSv1_2,
						},
						SslSecrets: &v1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      "solo",
								Namespace: "solo.io",
							},
						},
						SniDomains: []string{"b.com"},
					},
				})
				Expect(listener.GetFilterChains()).To(HaveLen(2))
				By("checking first filter chain")
				fc := listener.GetFilterChains()[0]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert := tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain1"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key1"))
				params := tlsContext(fc).GetCommonTlsContext().GetTlsParams()
				Expect(params.GetTlsMinimumProtocolVersion().String()).To(Equal("TLS_AUTO"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"a.com"}))
				By("checking second filter chain")
				fc = listener.GetFilterChains()[1]
				Expect(tlsContext(fc)).NotTo(BeNil())
				cert = tlsContext(fc).GetCommonTlsContext().GetTlsCertificates()[0]
				Expect(cert.GetCertificateChain().GetInlineString()).To(Equal("chain1"))
				Expect(cert.GetPrivateKey().GetInlineString()).To(Equal("key1"))
				params = tlsContext(fc).GetCommonTlsContext().GetTlsParams()
				Expect(params.GetTlsMinimumProtocolVersion().String()).To(Equal("TLSv1_2"))
				Expect(tlsContext(fc).GetCommonTlsContext().GetValidationContext()).To(BeNil())
				Expect(fc.FilterChainMatch.ServerNames).To(Equal([]string{"b.com"}))
				Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
			})
		})
	})

	It("Should report an error for virtual services with empty domains", func() {
		virtualHosts := []*v1.VirtualHost{
			{
				Domains: []string{"*"},
			},
			{
				Name:    "problem-vhost",
				Domains: []string{"", "nonempty-url.com"},
			},
		}
		report := &validation.HttpListenerReport{VirtualHostReports: []*validation.VirtualHostReport{{}, {}}}

		ValidateVirtualHostDomains(virtualHosts, report)

		Expect(report.VirtualHostReports[0].Errors).To(BeEmpty(), "The virtual host with domain * should not have an error")
		Expect(report.VirtualHostReports[1].Errors).NotTo(BeEmpty(), "The virtual host with an empty domain should report errors")
		Expect(report.VirtualHostReports[1].Errors[0].Type).To(Equal(validation.VirtualHostReport_Error_EmptyDomainError), "The error reported for the virtual host with empty domain should be the EmptyDomainError")
		Expect(listener.GetListenerFilters()[0].GetName()).To(Equal(wellknown.TlsInspector))
	})
})

// The endpoint Cluster is now the UpstreamToClusterName-<hash of upstream> to facilitate
// gRPC EDS updates
func getEndpointClusterName(upstream *v1.Upstream) string {
	return fmt.Sprintf("%s-%d", UpstreamToClusterName(upstream.Metadata.Ref()), upstream.MustHash())
}

func sv(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

type routePluginMock struct {
	ProcessRouteFunc func(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error
}

func (p *routePluginMock) Init(params plugins.InitParams) error {
	return nil
}

func (p *routePluginMock) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	return p.ProcessRouteFunc(params, in, out)
}

type endpointPluginMock struct {
	ProcessEndpointFunc func(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error
}

func (e *endpointPluginMock) ProcessEndpoints(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
	return e.ProcessEndpointFunc(params, in, out)
}

func (e *endpointPluginMock) Init(params plugins.InitParams) error {
	return nil
}
