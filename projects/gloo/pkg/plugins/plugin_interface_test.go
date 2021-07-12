package plugins

import (
	"sort"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/golang/protobuf/ptypes/any"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Plugin", func() {

	It("should order http filter stages correctly", func() {
		By("base case")
		filters := StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "mockFilter"}, BeforeStage(CorsStage)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"mockFilter"})

		By("before/after stage")
		filters = StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "A"}, BeforeStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "B"}, AfterStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "C"}, DuringStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "D"}, BeforeStage(CorsStage)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"A", "D", "C", "B"})

		By("all relative to the same well known stage, should order by weight and name")
		filters = StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "A"}, RelativeToStage(CorsStage, 5)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "B"}, RelativeToStage(CorsStage, 9)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "C"}, RelativeToStage(CorsStage, 0)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "D"}, RelativeToStage(CorsStage, -1)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"D", "C", "A", "B"})

		By("expected well known ordering")
		filters = StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "H"}, DuringStage(RouteStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "G"}, DuringStage(OutAuthStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "F"}, DuringStage(AcceptedStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "E"}, DuringStage(RateLimitStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "D"}, DuringStage(AuthZStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "C"}, DuringStage(AuthNStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "Waf"}, DuringStage(WafStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "B"}, DuringStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "A"}, DuringStage(FaultStage)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"A", "B", "Waf", "C", "D", "E", "F", "G", "H"})

		By("verify stable sort- check TypeUrl field")
		firstFilter := &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{TypeUrl: "a"}}}
		secondFilter := &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{TypeUrl: "b"}}}
		thirdFilter := &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{TypeUrl: "c"}}}
		filters = StagedHttpFilterList{
			StagedHttpFilter{firstFilter, DuringStage(RouteStage)},
			StagedHttpFilter{secondFilter, DuringStage(RouteStage)},
			StagedHttpFilter{thirdFilter, DuringStage(RouteStage)},
		}
		sort.Sort(filters)
		ExpectFilterConfigOrders(filters, []string{"a", "b", "c"}, []string{"", "", ""})

		By("verify stable sort- check Value field")
		firstFilter = &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{Value: []byte("a")}}}
		secondFilter = &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{Value: []byte("b")}}}
		thirdFilter = &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{Value: []byte("c")}}}
		filters = StagedHttpFilterList{
			StagedHttpFilter{firstFilter, DuringStage(RouteStage)},
			StagedHttpFilter{secondFilter, DuringStage(RouteStage)},
			StagedHttpFilter{thirdFilter, DuringStage(RouteStage)},
		}
		sort.Sort(filters)
		ExpectFilterConfigOrders(filters, []string{"", "", ""}, []string{"a", "b", "c"})

		By("verify stable sort- check both fields")
		firstFilter = &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{TypeUrl: "a", Value: []byte("b")}}}
		secondFilter = &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{TypeUrl: "a", Value: []byte("c")}}}
		thirdFilter = &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_TypedConfig{&any.Any{TypeUrl: "b", Value: []byte("a")}}}
		filters = StagedHttpFilterList{
			StagedHttpFilter{firstFilter, DuringStage(RouteStage)},
			StagedHttpFilter{secondFilter, DuringStage(RouteStage)},
			StagedHttpFilter{thirdFilter, DuringStage(RouteStage)},
		}
		sort.Sort(filters)
		ExpectFilterConfigOrders(filters, []string{"a", "a", "b"}, []string{"b", "c", "a"})
	})

	It("should order listener filter stages correctly", func() {
		By("base case")
		filters := StagedListenerFilterList{
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "H"}, DuringStage(RouteStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "G"}, DuringStage(OutAuthStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "F"}, DuringStage(AcceptedStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "E"}, DuringStage(RateLimitStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "D"}, DuringStage(AuthZStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "C"}, DuringStage(AuthNStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "Waf"}, DuringStage(WafStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "B"}, DuringStage(CorsStage)},
			StagedListenerFilter{&envoy_config_listener_v3.Filter{Name: "A"}, DuringStage(FaultStage)},
		}
		sort.Sort(filters)
		ExpectListenerFilterNameOrder(filters, []string{"A", "B", "Waf", "C", "D", "E", "F", "G", "H"})
	})
})

func ExpectListenerFilterNameOrder(filters StagedListenerFilterList, names []string) {
	Expect(len(filters)).To(Equal(len(names)))
	for i, filter := range filters {
		Expect(filter.ListenerFilter.Name).To(Equal(names[i]))
	}
}

func ExpectNameOrder(filters StagedHttpFilterList, names []string) {
	Expect(len(filters)).To(Equal(len(names)))
	for i, filter := range filters {
		Expect(filter.HttpFilter.Name).To(Equal(names[i]))
	}
}

func ExpectFilterConfigOrders(filters StagedHttpFilterList, typeUrls []string, values []string) {
	Expect(len(filters)).To(Equal(len(typeUrls)))
	Expect(len(filters)).To(Equal(len(values)))
	for i, filter := range filters {
		v := filter.HttpFilter.ConfigType.(*envoyhttp.HttpFilter_TypedConfig).TypedConfig
		Expect(v.TypeUrl).To(Equal(typeUrls[i]))
		Expect(string(v.Value)).To(Equal(values[i]))
	}
}
