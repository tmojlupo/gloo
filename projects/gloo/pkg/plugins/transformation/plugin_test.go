package transformation_test

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/any"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformers/xslt"
	matcherv3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("Plugin", func() {
	var (
		p               *Plugin
		expected        *any.Any
		outputTransform *envoytransformation.RouteTransformations
	)

	Context("translate transformations", func() {
		BeforeEach(func() {
			p = NewPlugin()
			err := p.Init(plugins.InitParams{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("translates header body transform", func() {
			headerBodyTransform := &envoytransformation.HeaderBodyTransform{}

			input := &transformation.Transformation{
				TransformationType: &transformation.Transformation_HeaderBodyTransform{
					HeaderBodyTransform: headerBodyTransform,
				},
			}

			expectedOutput := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_HeaderBodyTransform{
					HeaderBodyTransform: headerBodyTransform,
				},
			}
			output, err := p.TranslateTransformation(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(expectedOutput))
		})

		It("translates transformation template", func() {
			transformationTemplate := &envoytransformation.TransformationTemplate{
				HeadersToAppend: []*envoytransformation.TransformationTemplate_HeaderToAppend{
					{
						Key: "some-header",
						Value: &envoytransformation.InjaTemplate{
							Text: "some text",
						},
					},
				},
			}

			input := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: transformationTemplate,
				},
			}

			expectedOutput := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: transformationTemplate,
				},
			}
			output, err := p.TranslateTransformation(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(expectedOutput))
		})

		It("throws error on unsupported transformation type", func() {
			// Xslt Transformation is enterprise-only
			input := &transformation.Transformation{
				TransformationType: &transformation.Transformation_XsltTransformation{
					XsltTransformation: &xslt.XsltTransformation{
						Xslt: "<xsl:stylesheet>some transform</xsl:stylesheet>",
					},
				},
			}

			output, err := p.TranslateTransformation(input)
			Expect(output).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(UnknownTransformationType(&transformation.Transformation_XsltTransformation{})))

		})
	})

	Context("deprecated transformations", func() {
		var (
			inputTransform *transformation.Transformations
		)
		BeforeEach(func() {
			p = NewPlugin()
			err := p.Init(plugins.InitParams{})
			Expect(err).NotTo(HaveOccurred())
			inputTransform = &transformation.Transformations{
				ClearRouteCache: true,
			}
			outputTransform = &envoytransformation.RouteTransformations{
				// deprecated config gets old and new config
				ClearRouteCache: true,
				Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{ClearRouteCache: true},
						},
					},
				},
			}
			configStruct, err := utils.MessageToAny(outputTransform)
			Expect(err).NotTo(HaveOccurred())

			expected = configStruct
		})

		It("sets transformation config for weighted destinations", func() {
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
				Options: &v1.WeightedDestinationOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for virtual hosts", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for routes", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					Transformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets only one filter when no early filters exist", func() {
			filters, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(filters)).To(Equal(1))
			value := filters[0].HttpFilter.GetTypedConfig().GetValue()
			Expect(value).To(BeEmpty())
		})
	})

	Context("staged transformations", func() {
		var (
			inputTransform         *transformation.TransformationStages
			earlyStageFilterConfig *any.Any
		)
		BeforeEach(func() {
			p = NewPlugin()
			err := p.Init(plugins.InitParams{})
			Expect(err).NotTo(HaveOccurred())
			earlyStageFilterConfig, err = utils.MessageToAny(&envoytransformation.FilterTransformations{
				Stage: EarlyStageNumber,
			})
			Expect(err).NotTo(HaveOccurred())
			earlyRequestTransformationTemplate := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "1"},
				},
			}
			// construct transformation with all the options, to make sure translation is correct
			earlyRequestTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyRequestTransformationTemplate,
				},
			}
			envoyEarlyRequestTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyRequestTransformationTemplate,
				},
			}
			earlyResponseTransformationTemplate := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "2"},
				},
			}
			earlyResponseTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyResponseTransformationTemplate,
				},
			}
			envoyEarlyResponseTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: earlyResponseTransformationTemplate,
				},
			}
			requestTransformation := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "11"},
				},
			}
			requestTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: requestTransformation,
				},
			}
			envoyRequestTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: requestTransformation,
				},
			}
			responseTransformation := &envoytransformation.TransformationTemplate{
				AdvancedTemplates: true,
				BodyTransformation: &envoytransformation.TransformationTemplate_Body{
					Body: &envoytransformation.InjaTemplate{Text: "12"},
				},
			}
			responseTransform := &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: responseTransformation,
				},
			}
			envoyResponseTransform := &envoytransformation.Transformation{
				TransformationType: &envoytransformation.Transformation_TransformationTemplate{
					TransformationTemplate: responseTransformation,
				},
			}
			inputTransform = &transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{
						{
							Matchers: []*matchers.HeaderMatcher{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
							ResponseCodeDetails:    "abcd",
							ResponseTransformation: earlyResponseTransform,
						},
					},
					RequestTransforms: []*transformation.RequestMatch{
						{
							Matcher:                &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"}},
							ClearRouteCache:        true,
							RequestTransformation:  earlyRequestTransform,
							ResponseTransformation: earlyResponseTransform,
						},
					},
				},
				Regular: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{
						{
							Matchers: []*matchers.HeaderMatcher{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
							ResponseCodeDetails:    "abcd",
							ResponseTransformation: earlyResponseTransform,
						},
					},
					RequestTransforms: []*transformation.RequestMatch{
						{
							Matcher:                &matchers.Matcher{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo2"}},
							ClearRouteCache:        true,
							RequestTransformation:  requestTransform,
							ResponseTransformation: responseTransform,
						},
					},
				},
			}
			outputTransform = &envoytransformation.RouteTransformations{
				// new config should not get deprecated config
				Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
					{
						Stage: EarlyStageNumber,
						Match: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch_{
							ResponseMatch: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch{
								Match: &envoytransformation.ResponseMatcher{
									Headers: []*v3.HeaderMatcher{
										{
											Name:                 "foo",
											HeaderMatchSpecifier: &v3.HeaderMatcher_ExactMatch{ExactMatch: "bar"},
										},
									},
									ResponseCodeDetails: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "abcd"},
									},
								},
								ResponseTransformation: envoyEarlyResponseTransform,
							},
						},
					},
					{
						Stage: EarlyStageNumber,
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								Match:                  &v3.RouteMatch{PathSpecifier: &v3.RouteMatch_Prefix{Prefix: "/foo"}},
								ClearRouteCache:        true,
								RequestTransformation:  envoyEarlyRequestTransform,
								ResponseTransformation: envoyEarlyResponseTransform,
							},
						},
					},
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch_{
							ResponseMatch: &envoytransformation.RouteTransformations_RouteTransformation_ResponseMatch{
								Match: &envoytransformation.ResponseMatcher{
									Headers: []*v3.HeaderMatcher{
										{
											Name:                 "foo",
											HeaderMatchSpecifier: &v3.HeaderMatcher_ExactMatch{ExactMatch: "bar"},
										},
									},
									ResponseCodeDetails: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "abcd"},
									},
								},
								ResponseTransformation: envoyEarlyResponseTransform,
							},
						},
					},
					{
						Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
							RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
								Match:                  &v3.RouteMatch{PathSpecifier: &v3.RouteMatch_Prefix{Prefix: "/foo2"}},
								ClearRouteCache:        true,
								RequestTransformation:  envoyRequestTransform,
								ResponseTransformation: envoyResponseTransform,
							},
						},
					},
				},
			}
			configStruct, err := utils.MessageToAny(outputTransform)
			Expect(err).NotTo(HaveOccurred())

			expected = configStruct
		})
		It("sets transformation config for vhosts", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
				Options: &v1.VirtualHostOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for routes", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("sets transformation config for weighted dest", func() {
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
				Options: &v1.WeightedDestinationOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
		})
		It("should add both filter to the chain when early transformations exist", func() {
			out := &envoy_config_route_v3.Route{}
			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					StagedTransformations: inputTransform,
				},
			}, out)
			filters, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(filters)).To(Equal(2))
			value := filters[0].HttpFilter.GetTypedConfig()
			Expect(value).To(Equal(earlyStageFilterConfig))
			// second filter should have no stage, and thus empty config
			value = filters[1].HttpFilter.GetTypedConfig()
			Expect(value.GetValue()).To(BeEmpty())
		})
	})

})
