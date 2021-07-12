package cors

import (
	"strings"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Route Plugin", func() {
	var (
		params plugins.RouteParams
		plugin plugins.Plugin

		// values used in first example
		allowOrigin1      = []string{"solo.io", "github.com"}
		allowOriginRegex1 = []string{`*\.solo\.io`, `git.*\.com`}
		allowMethods1     = []string{"GET", "POST"}
		allowHeaders1     = []string{"allowH1", "allow2"}
		exposeHeaders1    = []string{"exHeader", "eh2"}
		maxAge1           = "5555"
	)

	BeforeEach(func() {
		plugin = NewPlugin()
		err := plugin.Init(plugins.InitParams{})
		Expect(err).NotTo(HaveOccurred())
		params = plugins.RouteParams{}

	})

	Context("CORS", func() {
		It("should full specification", func() {
			allowCredentials1 := true
			inRoute := routeWithCors(&cors.CorsPolicy{
				AllowOrigin:      allowOrigin1,
				AllowOriginRegex: allowOriginRegex1,
				AllowMethods:     allowMethods1,
				AllowHeaders:     allowHeaders1,
				ExposeHeaders:    exposeHeaders1,
				MaxAge:           maxAge1,
				AllowCredentials: allowCredentials1,
				DisableForRoute:  true,
			})
			outRoute := &envoy_config_route_v3.Route{
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{},
				},
			}
			expected := &envoy_config_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*envoy_type_matcher_v3.StringMatcher{
					&envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[0]},
					},
					&envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[1]},
					},
					&envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
								EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
								Regex:      allowOriginRegex1[0],
							},
						},
					},
					&envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
								EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
								Regex:      allowOriginRegex1[1],
							},
						},
					},
				},
				AllowMethods:     strings.Join(allowMethods1, ","),
				AllowHeaders:     strings.Join(allowHeaders1, ","),
				ExposeHeaders:    strings.Join(exposeHeaders1, ","),
				MaxAge:           maxAge1,
				AllowCredentials: &wrappers.BoolValue{Value: allowCredentials1},
				EnabledSpecifier: &envoy_config_route_v3.CorsPolicy_FilterEnabled{
					FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
						DefaultValue: &envoy_type_v3.FractionalPercent{
							Numerator:   0,
							Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
						},
						RuntimeKey: runtimeKey,
					},
				},
			}

			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outRoute.Action.(*envoy_config_route_v3.Route_Route).Route.Cors).To(Equal(expected))
		})
		It("should process  minimal specification", func() {
			inRoute := routeWithCors(&cors.CorsPolicy{
				AllowOrigin: allowOrigin1,
			})
			outRoute := basicEnvoyRoute()
			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).NotTo(HaveOccurred())
			cSpec := &envoy_config_route_v3.CorsPolicy{
				AllowOriginStringMatch: []*envoy_type_matcher_v3.StringMatcher{
					&envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[0]},
					},
					&envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: allowOrigin1[1]},
					},
				},
			}
			expected := basicEnvoyRouteWithCors(cSpec)
			Expect(outRoute.Action.(*envoy_config_route_v3.Route_Route).Route.Cors).To(Equal(cSpec))
			Expect(outRoute).To(Equal(expected))
		})
		It("should process empty specification", func() {
			inRoute := routeWithCors(&cors.CorsPolicy{})
			outRoute := basicEnvoyRoute()
			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).To(HaveOccurred())
			cSpec := &envoy_config_route_v3.CorsPolicy{}
			expected := basicEnvoyRouteWithCors(cSpec)
			Expect(outRoute.Action.(*envoy_config_route_v3.Route_Route).Route.Cors).To(Equal(cSpec))
			Expect(outRoute.String()).To(Equal(expected.String()))
			Expect(outRoute).To(Equal(expected))
		})
		It("should process null specification", func() {
			inRoute := routeWithCors(nil)
			outRoute := basicEnvoyRoute()
			err := plugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
			Expect(err).NotTo(HaveOccurred())
			expected := basicEnvoyRoute()
			Expect(outRoute).To(Equal(expected))
		})
	})

})

func routeWithoutCors() *v1.Route {
	return &v1.Route{
		Action: &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &core.ResourceRef{
								Name:      "test",
								Namespace: "default",
							},
						},
					},
				},
			},
		},
	}
}

func routeWithCors(cSpec *cors.CorsPolicy) *v1.Route {
	route := routeWithoutCors()
	route.Options = &v1.RouteOptions{
		Cors: cSpec,
	}
	return route
}

func basicEnvoyRoute() *envoy_config_route_v3.Route {
	return &envoy_config_route_v3.Route{
		Action: &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{},
		},
	}
}

func basicEnvoyRouteWithCors(cSpec *envoy_config_route_v3.CorsPolicy) *envoy_config_route_v3.Route {
	return &envoy_config_route_v3.Route{
		Action: &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{
				Cors: cSpec,
			},
		},
	}
}
