package consul

import (
	"net"
	"net/url"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	mock_consul2 "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul/mocks"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
)

var _ = Describe("Resolve", func() {
	var (
		ctrl              *gomock.Controller
		consulWatcherMock *mock_consul.MockConsulWatcher
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)

		consulWatcherMock = mock_consul.NewMockConsulWatcher(ctrl)

	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("can resolve consul service addresses that are IPs", func() {
		plug := NewPlugin(consulWatcherMock, nil, nil)

		svcName := "my-svc"
		tag := "tag"
		dc := "dc1"

		us := createTestFilteredUpstream(svcName, svcName, nil, []string{tag}, []string{dc})

		queryOpts := &consulapi.QueryOptions{Datacenter: dc, RequireConsistent: true}

		consulWatcherMock.EXPECT().Service(svcName, "", queryOpts).Return([]*consulapi.CatalogService{
			{
				ServiceAddress: "5.6.7.8",
				ServicePort:    1234,
			},
			{
				ServiceAddress: "1.2.3.4",
				ServicePort:    1234,
				ServiceTags:    []string{tag},
			},
		}, nil, nil)

		u, err := plug.Resolve(us)
		Expect(err).NotTo(HaveOccurred())

		Expect(u).To(Equal(&url.URL{Scheme: "http", Host: "1.2.3.4:1234"}))
	})

	It("can resolve consul service addresses that are hostnames", func() {

		ips := []net.IPAddr{
			{IP: net.IPv4(2, 1, 0, 10)}, // we will arbitrarily default to the first DNS response
			{IP: net.IPv4(2, 1, 0, 11)},
		}
		mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
		mockDnsResolver.EXPECT().Resolve(gomock.Any(), "test.service.consul").Return(ips, nil).Times(1)

		plug := NewPlugin(consulWatcherMock, mockDnsResolver, nil)

		svcName := "my-svc"
		tag := "tag"
		dc := "dc1"

		us := createTestFilteredUpstream(svcName, svcName, nil, []string{tag}, []string{dc})

		queryOpts := &consulapi.QueryOptions{Datacenter: dc, RequireConsistent: true}

		consulWatcherMock.EXPECT().Service(svcName, "", queryOpts).Return([]*consulapi.CatalogService{
			{
				ServiceAddress: "5.6.7.8",
				ServicePort:    1234,
			},
			{
				ServiceAddress: "test.service.consul",
				ServicePort:    1234,
				ServiceTags:    []string{tag},
			},
		}, nil, nil)

		u, err := plug.Resolve(us)
		Expect(err).NotTo(HaveOccurred())

		Expect(u).To(Equal(&url.URL{Scheme: "http", Host: "2.1.0.10:1234"}))
	})

	It("can resolve consul service addresses in an unfiltered upstream", func() {

		plug := NewPlugin(consulWatcherMock, nil, nil)

		svcName := "my-svc"
		dc := "dc1"

		us := createTestFilteredUpstream(svcName, svcName, nil, nil, []string{dc})

		queryOpts := &consulapi.QueryOptions{Datacenter: dc, RequireConsistent: true}

		consulWatcherMock.EXPECT().Service(svcName, "", queryOpts).Return([]*consulapi.CatalogService{
			{
				ServiceAddress: "5.6.7.8",
				ServicePort:    1234,
			},
		}, nil, nil)

		u, err := plug.Resolve(us)
		Expect(err).NotTo(HaveOccurred())

		Expect(u).To(Equal(&url.URL{Scheme: "http", Host: "5.6.7.8:1234"}))
	})

	It("properly initializes with a detailed upstream discovery config.", func() {

		// correct w/custom tag
		plug := NewPlugin(consulWatcherMock, nil, nil)
		err := plug.Init(plugins.InitParams{
			Settings: &v1.Settings{ConsulDiscovery: &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
				UseTlsTagging: true,
				TlsTagName:    "testTag",
				RootCa: &core.ResourceRef{
					Namespace: "rootNs",
					Name:      "rootName",
				},
			},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(plug.consulUpstreamDiscoverySettings.TlsTagName).To(Equal("testTag"))
		Expect(plug.consulUpstreamDiscoverySettings.RootCa.Namespace).To(Equal("rootNs"))
		Expect(plug.consulUpstreamDiscoverySettings.RootCa.Name).To(Equal("rootName"))
	})

	It("properly uses the default tls tag if it's not set in the input config.", func() {

		// correct w/default tag
		plug := NewPlugin(consulWatcherMock, nil, nil)
		err := plug.Init(plugins.InitParams{
			Settings: &v1.Settings{ConsulDiscovery: &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
				UseTlsTagging: true,
				RootCa: &core.ResourceRef{
					Namespace: "rootNs",
					Name:      "rootName",
				},
			},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(plug.consulUpstreamDiscoverySettings.TlsTagName).To(Equal(DefaultTlsTagName))
	})

	It("returns an error if it tries to init with missing required values.", func() {
		// missing resource value, expect err.
		plug := NewPlugin(consulWatcherMock, nil, nil)
		var rootCa = &core.ResourceRef{
			Namespace: "rootNs",
			Name:      "",
		}
		err := plug.Init(plugins.InitParams{
			Settings: &v1.Settings{ConsulDiscovery: &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
				UseTlsTagging: true,
				RootCa:        rootCa,
			},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ConsulTlsInputError(rootCa.String()).Error()))
	})
})
