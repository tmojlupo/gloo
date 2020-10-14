package translator_test

import (
	"context"
	"fmt"

	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyrouteapi "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	gloo_envoy_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	consul2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/solo-io/gloo/pkg/utils"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	v1kubernetes "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Translator", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		upstream          *v1.Upstream
		upName            core.Metadata
		proxy             *v1.Proxy
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		matcher           *matchers.Matcher
		routes            []*v1.Route

		snapshot           envoycache.Snapshot
		cluster            *envoyapi.Cluster
		listener           *envoyapi.Listener
		endpoints          envoycache.Resources
		hcmCfg             *envoyhttp.HttpConnectionManager
		routeConfiguration *envoyapi.RouteConfiguration
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

		upName = core.Metadata{
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
						Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(upName.Ref())},
						Address:   "1.2.3.4",
						Port:      32,
						Metadata: core.Metadata{
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
								Upstream: utils.ResourceRefPtr(upName.Ref()),
							},
						},
					},
				},
			},
		}}
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
						Name:    "virt1",
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
						},
					},
				},
			},
		}
		proxy = &v1.Proxy{
			Metadata: core.Metadata{
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
		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).To(HaveOccurred())
		return report
	}

	translate := func() {
		snap, errs, report, err := translator.Translate(params, proxy)
		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		clusters := snap.GetResources(xds.ClusterType)
		clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
		cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
		Expect(cluster).NotTo(BeNil())

		listeners := snap.GetResources(xds.ListenerType)
		listenerResource := listeners.Items["http-listener"]
		listener = listenerResource.ResourceProto().(*envoyapi.Listener)
		Expect(listener).NotTo(BeNil())

		hcmFilter := listener.FilterChains[0].Filters[0]
		hcmCfg = &envoyhttp.HttpConnectionManager{}
		err = ParseTypedConfig(hcmFilter, hcmCfg)
		Expect(err).NotTo(HaveOccurred())

		routes := snap.GetResources(xds.RouteType)
		Expect(routes.Items).To(HaveKey("http-listener-routes"))
		routeResource := routes.Items["http-listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())

		endpoints = snap.GetResources(xds.EndpointType)

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

		routes := snap.GetResources(xds.RouteType)
		Expect(routes.Items).To(HaveKey("http-listener-routes"))
		routeResource := routes.Items["http-listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())
		Expect(routeConfiguration.GetVirtualHosts()).To(HaveLen(1))
		Expect(routeConfiguration.GetVirtualHosts()[0].Name).To(Equal("invalid_name"))
	})

	It("translates listener options", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)

		proxyClone.GetListeners()[0].Options = &v1.ListenerOptions{PerConnectionBufferLimitBytes: &types.UInt32Value{Value: 4096}}

		snap, errs, report, err := translator.Translate(params, proxyClone)

		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())
		Expect(report).To(Equal(validationutils.MakeReport(proxy)))

		listeners := snap.GetResources(xds.ListenerType)
		Expect(listeners.Items).To(HaveKey("http-listener"))
		listenerResource := listeners.Items["http-listener"]
		listenerConfiguration := listenerResource.ResourceProto().(*envoyapi.Listener)
		Expect(listenerConfiguration).NotTo(BeNil())
		Expect(listenerConfiguration.PerConnectionBufferLimitBytes).To(Equal(&wrappers.UInt32Value{Value: 4096}))
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
			Expect(errs.Validate().Error()).To(ContainSubstring("HttpListener Error: ProcessingError. Reason: auth config not found:"))
		})
	})

	Context("service spec", func() {
		It("changes in service spec should create a different snapshot", func() {
			translate()
			oldVersion := snapshot.GetResources(xds.ClusterType).Version

			svcSpec := &v1plugins.ServiceSpec{
				PluginType: &v1plugins.ServiceSpec_Grpc{
					Grpc: &v1grpc.ServiceSpec{},
				},
			}
			upstream.UpstreamType.(*v1.Upstream_Static).SetServiceSpec(svcSpec)
			translate()
			newVersion := snapshot.GetResources(xds.ClusterType).Version
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
			Expect(errs.Validate().Error()).To(ContainSubstring("Route Error: InvalidMatcherError. Reason: no path specifier provided"))
			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
				{
					Type:   validation.RouteReport_Error_InvalidMatcherError,
					Reason: "no path specifier provided",
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
			Expect(errs.Validate().Error()).To(ContainSubstring("Route Error: InvalidMatcherError. Reason: no path specifier provided; Route Error: ProcessingError. Reason: *grpc.plugin: missing path for grpc route"))

			expectedReport := validationutils.MakeReport(proxy)
			expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
				{
					Type:   validation.RouteReport_Error_InvalidMatcherError,
					Reason: "no path specifier provided",
				},
				{
					Type:   validation.RouteReport_Error_ProcessingError,
					Reason: "*grpc.plugin: missing path for grpc route",
				},
			}
			Expect(report).To(Equal(expectedReport))
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
					RegexMaxProgramSize: &types.UInt32Value{Value: 200},
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
				fooRoute.Match = &envoyrouteapi.RouteMatch{}
				barRoute.Match = &envoyrouteapi.RouteMatch{}
				fooRoute.Name = ""
				barRoute.Name = ""

				Expect(fooRoute).To(Equal(barRoute))
			})
		})
	})

	Context("Health check config", func() {

		It("will error if required field is nil", func() {
			upstream.HealthChecks = []*gloo_envoy_core.HealthCheck{
				{
					Interval: &DefaultHealthCheckInterval,
				},
			}
			report := translateWithError()

			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("will error if no health checker is supplied", func() {
			upstream.HealthChecks = []*gloo_envoy_core.HealthCheck{
				{
					Timeout:            &DefaultHealthCheckTimeout,
					Interval:           &DefaultHealthCheckInterval,
					HealthyThreshold:   DefaultThreshold,
					UnhealthyThreshold: DefaultThreshold,
				},
			}
			report := translateWithError()
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))
		})

		It("can translate the http health check", func() {
			expectedResult := []*envoycore.HealthCheck{
				{
					Timeout:            gogoutils.DurationStdToProto(&DefaultHealthCheckTimeout),
					Interval:           gogoutils.DurationStdToProto(&DefaultHealthCheckInterval),
					HealthyThreshold:   gogoutils.UInt32GogoToProto(DefaultThreshold),
					UnhealthyThreshold: gogoutils.UInt32GogoToProto(DefaultThreshold),
					HealthChecker: &envoycore.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &envoycore.HealthCheck_HttpHealthCheck{
							Host:                   "host",
							Path:                   "path",
							ServiceName:            "svc",
							RequestHeadersToAdd:    []*envoycore.HeaderValueOption{},
							RequestHeadersToRemove: []string{},
							UseHttp2:               true,
							ExpectedStatuses:       []*envoy_type.Int64Range{},
						},
					},
				},
			}
			var err error
			upstream.HealthChecks, err = gogoutils.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			translate()
			Expect(cluster.HealthChecks).To(BeEquivalentTo(expectedResult))
		})

		It("can translate the grpc health check", func() {
			expectedResult := []*envoycore.HealthCheck{
				{
					Timeout:            gogoutils.DurationStdToProto(&DefaultHealthCheckTimeout),
					Interval:           gogoutils.DurationStdToProto(&DefaultHealthCheckInterval),
					HealthyThreshold:   gogoutils.UInt32GogoToProto(DefaultThreshold),
					UnhealthyThreshold: gogoutils.UInt32GogoToProto(DefaultThreshold),
					HealthChecker: &envoycore.HealthCheck_GrpcHealthCheck_{
						GrpcHealthCheck: &envoycore.HealthCheck_GrpcHealthCheck{
							ServiceName: "svc",
							Authority:   "authority",
						},
					},
				},
			}
			var err error
			upstream.HealthChecks, err = gogoutils.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())
			translate()
			Expect(cluster.HealthChecks).To(BeEquivalentTo(expectedResult))
		})

		It("can properly translate outlier detection config", func() {
			dur := &duration.Duration{Seconds: 1}
			expectedResult := &envoycluster.OutlierDetection{
				Consecutive_5Xx:                        gogoutils.UInt32GogoToProto(DefaultThreshold),
				Interval:                               dur,
				BaseEjectionTime:                       dur,
				MaxEjectionPercent:                     gogoutils.UInt32GogoToProto(DefaultThreshold),
				EnforcingConsecutive_5Xx:               gogoutils.UInt32GogoToProto(DefaultThreshold),
				EnforcingSuccessRate:                   gogoutils.UInt32GogoToProto(DefaultThreshold),
				SuccessRateMinimumHosts:                gogoutils.UInt32GogoToProto(DefaultThreshold),
				SuccessRateRequestVolume:               gogoutils.UInt32GogoToProto(DefaultThreshold),
				SuccessRateStdevFactor:                 nil,
				ConsecutiveGatewayFailure:              gogoutils.UInt32GogoToProto(DefaultThreshold),
				EnforcingConsecutiveGatewayFailure:     nil,
				SplitExternalLocalOriginErrors:         true,
				ConsecutiveLocalOriginFailure:          nil,
				EnforcingConsecutiveLocalOriginFailure: nil,
				EnforcingLocalOriginSuccessRate:        nil,
			}
			upstream.OutlierDetection = gogoutils.ToGlooOutlierDetection(expectedResult)
			translate()
			Expect(cluster.OutlierDetection).To(BeEquivalentTo(expectedResult))
		})

		It("can properly validate outlier detection config", func() {
			expectedResult := &envoycluster.OutlierDetection{}
			upstream.OutlierDetection = gogoutils.ToGlooOutlierDetection(expectedResult)
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
					Metadata: core.Metadata{
						Name:      "foo",
						Namespace: "bar",
					},
				},
			}

			expectedResult := []*envoycore.HealthCheck{
				{
					Timeout:            gogoutils.DurationStdToProto(&DefaultHealthCheckTimeout),
					Interval:           gogoutils.DurationStdToProto(&DefaultHealthCheckInterval),
					HealthyThreshold:   gogoutils.UInt32GogoToProto(DefaultThreshold),
					UnhealthyThreshold: gogoutils.UInt32GogoToProto(DefaultThreshold),
					HealthChecker: &envoycore.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &envoycore.HealthCheck_HttpHealthCheck{
							Host:                   "host",
							Path:                   "path",
							ServiceName:            "svc",
							RequestHeadersToAdd:    []*envoycore.HeaderValueOption{},
							RequestHeadersToRemove: []string{},
							UseHttp2:               true,
							ExpectedStatuses:       []*envoy_type.Int64Range{},
						},
					},
				},
			}

			var err error
			upstream.HealthChecks, err = gogoutils.ToGlooHealthCheckList(expectedResult)
			Expect(err).NotTo(HaveOccurred())

			expectedResult[0].GetHttpHealthCheck().RequestHeadersToAdd = []*envoycore.HeaderValueOption{
				{
					Header: &envoycore.HeaderValue{
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
					Append: &types.BoolValue{
						Value: true,
					},
				},
			}

			snap, errs, report, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())
			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(snap).NotTo(BeNil())
			Expect(report).To(Equal(validationutils.MakeReport(proxy)))

			clusters := snap.GetResources(xds.ClusterType)
			clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.HealthChecks).To(BeEquivalentTo(expectedResult))
		})
	})

	Context("circuit breakers", func() {

		It("should NOT translate circuit breakers on upstream", func() {
			translate()
			Expect(cluster.CircuitBreakers).To(BeNil())
		})

		It("should translate circuit breakers on upstream", func() {

			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

		It("should translate circuit breakers on settings", func() {

			settings.Gloo = &v1.GlooOptions{}
			settings.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

		It("should override circuit breakers on upstream", func() {

			settings.Gloo = &v1.GlooOptions{}
			settings.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 11},
				MaxPendingRequests: &types.UInt32Value{Value: 12},
				MaxRequests:        &types.UInt32Value{Value: 13},
				MaxRetries:         &types.UInt32Value{Value: 14},
			}

			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &wrappers.UInt32Value{Value: 1},
						MaxPendingRequests: &wrappers.UInt32Value{Value: 2},
						MaxRequests:        &wrappers.UInt32Value{Value: 3},
						MaxRetries:         &wrappers.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})
	})

	Context("eds", func() {

		It("should translate eds differently with different clusters", func() {
			translate()
			version1 := endpoints.Version
			// change the cluster
			upstream.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxRetries: &types.UInt32Value{Value: 5},
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
					Metadata: core.Metadata{
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
			localUpstream2 = buildLocalUpstream("randomString")

		})

		It("should have same version and http filters when http filters with the same name are added in a different order", func() {
			translate()

			By("get the original version and http filters")

			// get version
			originalVersion := snapshot.GetResources(xds.ListenerType).Version

			// get http filters
			hcmFilter := listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err := glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoylistener.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			originalHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters

			By("add the upstreams and compare the new version and http filters")

			// add upstreams with same name
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream1)
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, localUpstream2)
			Expect(params.Snapshot.Upstreams).To(HaveLen(3))

			translate()

			// get and compare version
			upstreamsVersion := snapshot.GetResources(xds.ListenerType).Version
			Expect(upstreamsVersion).ToNot(Equal(originalVersion))

			// get and compare http filters
			hcmFilter = listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err = glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoylistener.Filter_TypedConfig).TypedConfig)
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
			flipOrderVersion := snapshot.GetResources(xds.ListenerType).Version
			Expect(flipOrderVersion).To(Equal(upstreamsVersion))

			// get and compare http filters
			hcmFilter = listener.GetFilterChains()[0].GetFilters()[0]
			typedConfig, err = glooutils.AnyToMessage(hcmFilter.GetConfigType().(*envoylistener.Filter_TypedConfig).TypedConfig)
			Expect(err).NotTo(HaveOccurred())
			flipOrderHttpFilters := typedConfig.(*envoyhttp.HttpConnectionManager).HttpFilters
			Expect(flipOrderHttpFilters).To(Equal(upstreamsHttpFilters))
		})

	})

	Context("when handling upstream groups", func() {

		var (
			upstream2     *v1.Upstream
			upstreamGroup *v1.UpstreamGroup
		)

		BeforeEach(func() {
			upstream2 = &v1.Upstream{
				Metadata: core.Metadata{
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
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "gloo-system",
				},
				Destinations: []*v1.WeightedDestination{
					{
						Weight: 1,
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
							},
						},
					},
					{
						Weight: 1,
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(upstream2.Metadata.Ref()),
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
							UpstreamGroup: &ref,
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
					Reason: "invalid destination in weighted destination list: *v1.Upstream {notexist gloo-system} not found",
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

	Context("when handling endpoints", func() {
		var (
			claConfiguration *envoyapi.ClusterLoadAssignment
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
					Metadata: core.Metadata{
						Name:        "test",
						Namespace:   "gloo-system",
						Annotations: annotations,
					},
					Upstreams: []*core.ResourceRef{
						&ref,
					},
					Address: "1.2.3.4",
					Port:    1234,
				},
			}
		})
		It("should transfer annotations to snapshot", func() {
			translate()

			endpoints := snapshot.GetResources(xds.EndpointType)

			clusterName := UpstreamToClusterName(upstream.Metadata.Ref())
			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoyapi.ClusterLoadAssignment)
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
			claConfiguration *envoyapi.ClusterLoadAssignment
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
					Metadata: core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
						Labels:    map[string]string{"testkey": "testvalue"},
					},
					Upstreams: []*core.ResourceRef{
						&ref,
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
									Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
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

			endpoints := snapshot.GetResources(xds.EndpointType)

			clusterName := UpstreamToClusterName(upstream.Metadata.Ref())
			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoyapi.ClusterLoadAssignment)
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
				Expect(fields).To(HaveKeyWithValue("testkey", sv("testvalue")))
			})

			It("should add subset to cluster", func() {
				translateWithEndpoints()

				Expect(cluster.LbSubsetConfig).ToNot(BeNil())
				Expect(cluster.LbSubsetConfig.FallbackPolicy).To(Equal(envoyapi.Cluster_LbSubsetConfig_ANY_ENDPOINT))
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
			Expect(fields).To(HaveKeyWithValue("testkey", sv("")))
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
										Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
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
				expectedReport := validationutils.MakeReport(proxy)
				expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Errors = []*validation.RouteReport_Error{
					{
						Type:   validation.RouteReport_Error_ProcessingError,
						Reason: "route has a subset config, but none of the subsets in the upstream match it.",
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
										Upstream: &core.ResourceRef{"do", "notexist"},
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
				Expect(errs.ValidateStrict().Error()).To(ContainSubstring("*v1.Upstream {do notexist} not found"))
				expectedReport := validationutils.MakeReport(proxy)
				expectedReport.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[0].RouteReports[0].Warnings = []*validation.RouteReport_Warning{
					{
						Type:   validation.RouteReport_Warning_InvalidDestinationWarning,
						Reason: "*v1.Upstream {do notexist} not found",
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
					Metadata: core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.1", svc.Spec.Ports[0].Port),
					},
					Port:      uint32(svc.Spec.Ports[0].Port),
					Address:   "192.168.0.1",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.2", svc.Spec.Ports[1].Port),
					},
					Port:      uint32(svc.Spec.Ports[1].Port),
					Address:   "192.168.0.2",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[1].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: core.ResourceRef{
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
			clusters := snapshot.GetResources(xds.ClusterType)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())
			clusterResource = clusters.Items[UpstreamToClusterName(fakeUsList[1].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())

			// A route to the kube service has been configured
			routes := snapshot.GetResources(xds.RouteType)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoyrouteapi.Route_Route)
			Expect(ok).To(BeTrue())
			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoyrouteapi.RouteAction_Cluster)
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
					Metadata: core.Metadata{
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
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: core.Metadata{
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
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: core.Metadata{
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
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
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
			clusters := snapshot.GetResources(xds.ClusterType)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(3))
			// Order is important here
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(ConsistOf(
				&envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west)},
				},
				&envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{tag(dev), tag(prod)},
				},
				&envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west), tag(dev), tag(prod)},
				},
			))

			// A route to the kube service has been configured
			routes := snapshot.GetResources(xds.RouteType)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoyrouteapi.Route_Route)
			Expect(ok).To(BeTrue())

			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoyrouteapi.RouteAction_Cluster)
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
			routePlugin.ProcessRouteFunc = func(params plugins.RouteParams, in *v1.Route, out *envoyrouteapi.Route) error {
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
			additionalEndpoint := &envoy_api_v2_endpoint.LocalityLbEndpoints{
				Locality: &envoycore.Locality{
					Region: "region",
					Zone:   "a",
				},
				Priority: 10,
			}

			endpointPlugin.ProcessEndpointFunc = func(params plugins.Params, in *v1.Upstream, out *envoyapi.ClusterLoadAssignment) error {
				Expect(out.GetEndpoints()).To(HaveLen(1))
				Expect(out.GetClusterName()).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
				Expect(out.GetEndpoints()[0].GetLbEndpoints()).To(HaveLen(1))

				out.Endpoints = append(out.Endpoints, additionalEndpoint)
				return nil
			}

			translate()
			endpointResource := endpoints.Items["test_gloo-system"]
			endpoint := endpointResource.ResourceProto().(*envoyapi.ClusterLoadAssignment)
			Expect(endpoint).NotTo(BeNil())
			Expect(endpoint.Endpoints).To(HaveLen(2))
			Expect(endpoint.Endpoints[1]).To(Equal(additionalEndpoint))
		})

		It("should call the endpoint plugin with an empty endpoint", func() {
			// Create an empty consul upstream just to get EDS
			emptyUpstream := &v1.Upstream{
				Metadata: core.Metadata{
					Namespace: "empty_namespace",
					Name:      "empty_name",
				},
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul2.UpstreamSpec{},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, emptyUpstream)

			foundEmptyUpstream := false

			endpointPlugin.ProcessEndpointFunc = func(params plugins.Params, in *v1.Upstream, out *envoyapi.ClusterLoadAssignment) error {
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
							Append: &types.BoolValue{
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
			routes := snapshot.GetResources(xds.RouteType)
			Expect(routes.Items).To(HaveKey("http-listener-routes"))
			routeResource := routes.Items["http-listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))

			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Routes[0].ResponseHeadersToAdd).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Routes[0].ResponseHeadersToAdd).To(ConsistOf(
				&envoycore.HeaderValueOption{
					Header: &envoycore.HeaderValue{
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
			listeners := snapshot.GetResources(xds.ListenerType).Items
			Expect(listeners).NotTo(HaveLen(0))
			val, found := listeners["tcp-listener"]
			Expect(found).To(BeTrue())
			listener, ok := val.ResourceProto().(*envoyapi.Listener)
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
		})
	})

	Context("Ssl", func() {

		var (
			listener *envoyapi.Listener
		)

		prep := func(s []*v1.SslConfig) {

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
			translate()

			listeners := snapshot.GetResources(xds.ListenerType).Items
			Expect(listeners).To(HaveLen(1))
			val, found := listeners["http-listener"]
			Expect(found).To(BeTrue())
			listener = val.ResourceProto().(*envoyapi.Listener)
		}
		tlsContext := func(fc *envoylistener.FilterChain) *envoyauth.DownstreamTlsContext {
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
			})

			It("should reject configs if different FilterChains have identical FilterChainMatches", func() {
				filterChains := []*envoylistener.FilterChain{
					{
						FilterChainMatch: &envoylistener.FilterChainMatch{
							DestinationPort: &wrappers.UInt32Value{Value: 1},
						},
					},
					{
						FilterChainMatch: &envoylistener.FilterChainMatch{
							DestinationPort: &wrappers.UInt32Value{Value: 1},
						},
					},
				}
				report := &validation.ListenerReport{}
				CheckForDuplicateFilterChainMatches(filterChains, report)
				Expect(report.Errors).NotTo(BeNil())
				Expect(report.Errors).To(HaveLen(1))
				Expect(report.Errors[0].Type).To(Equal(validation.ListenerReport_Error_SSLConfigError))
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
			})
		})
		Context("secret refs", func() {
			It("should combine sni matches ", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: core.Metadata{
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
			})
			It("should not combine when not matching", func() {

				params.Snapshot.Secrets = append(params.Snapshot.Secrets, &v1.Secret{
					Metadata: core.Metadata{
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
					Metadata: core.Metadata{
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
					Metadata: core.Metadata{
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
				})

				Expect(listener.GetFilterChains()).To(HaveLen(3))
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
	})
})

func sv(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

type routePluginMock struct {
	ProcessRouteFunc func(params plugins.RouteParams, in *v1.Route, out *envoyrouteapi.Route) error
}

func (p *routePluginMock) Init(params plugins.InitParams) error {
	return nil
}

func (p *routePluginMock) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyrouteapi.Route) error {
	return p.ProcessRouteFunc(params, in, out)
}

type endpointPluginMock struct {
	ProcessEndpointFunc func(params plugins.Params, in *v1.Upstream, out *envoyapi.ClusterLoadAssignment) error
}

func (e *endpointPluginMock) ProcessEndpoints(params plugins.Params, in *v1.Upstream, out *envoyapi.ClusterLoadAssignment) error {
	return e.ProcessEndpointFunc(params, in, out)
}

func (e *endpointPluginMock) Init(params plugins.InitParams) error {
	return nil
}
