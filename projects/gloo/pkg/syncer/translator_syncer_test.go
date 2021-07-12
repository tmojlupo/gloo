package syncer_test

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var _ = Describe("Translate Proxy", func() {

	var (
		xdsCache       *MockXdsCache
		sanitizer      *MockXdsSanitizer
		syncer         v1.ApiSyncer
		snap           *v1.ApiSnapshot
		settings       *v1.Settings
		upstreamClient clients.ResourceClient
		proxyClient    v1.ProxyClient
		ctx            context.Context
		cancel         context.CancelFunc
		proxyName      = "proxy-name"
		ref            = "syncer-test"
		ns             = "any-ns"
	)

	BeforeEach(func() {
		var err error
		xdsCache = &MockXdsCache{}
		sanitizer = &MockXdsSanitizer{}
		ctx, cancel = context.WithCancel(context.Background())

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(ctx, resourceClientFactory)

		upstreamClient, err = resourceClientFactory.NewResourceClient(ctx, factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName,
			},
		}

		settings = &v1.Settings{}

		rep := reporter.NewReporter(ref, proxyClient.BaseClient(), upstreamClient)

		xdsHasher := &xds.ProxyKeyHasher{}
		syncer = NewTranslatorSyncer(&mockTranslator{true, false, nil}, xdsCache, xdsHasher, sanitizer, rep, false, nil, settings)
		snap = &v1.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy,
			},
		}
		_, err = proxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		proxies, err := proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].Status).To(Equal(&core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
			ReportedBy: ref,
		}))

		// NilSnapshot is always consistent, so snapshot will always be set as part of endpoints update
		Expect(xdsCache.Called).To(BeTrue())

		// update rv for proxy
		p1, err := proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		snap.Proxies[0] = p1

		syncer = NewTranslatorSyncer(&mockTranslator{false, false, nil}, xdsCache, xdsHasher, sanitizer, rep, false, nil, settings)

		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterEach(func() { cancel() })

	It("writes the reports the translator spits out and calls SetSnapshot on the cache", func() {
		proxies, err := proxyClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].Status).To(Equal(&core.Status{
			State:      1,
			ReportedBy: ref,
		}))

		Expect(xdsCache.Called).To(BeTrue())
	})

	It("updates the cache with the sanitized snapshot", func() {
		sanitizer.Snap = envoycache.NewEasyGenericSnapshot("easy")
		err := syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Expect(sanitizer.Called).To(BeTrue())
		Expect(xdsCache.SetSnap).To(BeEquivalentTo(sanitizer.Snap))
	})

	It("uses listeners and routes from the previous snapshot when sanitization fails", func() {
		sanitizer.Err = errors.Errorf("we ran out of coffee")

		oldXdsSnap := xds.NewSnapshotFromResources(
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
			envoycache.NewResources("old listeners from before the war", []envoycache.Resource{
				resource.NewEnvoyResource(&envoy_config_listener_v3.Listener{}),
			}),
		)

		// return this old snapshot when the syncer asks for it
		xdsCache.GetSnap = oldXdsSnap
		err := syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Expect(sanitizer.Called).To(BeTrue())
		Expect(xdsCache.Called).To(BeTrue())

		oldListeners := oldXdsSnap.GetResources(resource.ListenerTypeV3)
		newListeners := xdsCache.SetSnap.GetResources(resource.ListenerTypeV3)

		Expect(oldListeners).To(Equal(newListeners))

		oldRoutes := oldXdsSnap.GetResources(resource.RouteTypeV3)
		newRoutes := xdsCache.SetSnap.GetResources(resource.RouteTypeV3)

		Expect(oldRoutes).To(Equal(newRoutes))
	})

})

var _ = Describe("Empty cache", func() {

	var (
		xdsCache       *MockXdsCache
		sanitizer      *MockXdsSanitizer
		syncer         v1.ApiSyncer
		settings       *v1.Settings
		upstreamClient clients.ResourceClient
		proxyClient    v1.ProxyClient
		ctx            context.Context
		cancel         context.CancelFunc
		proxy          *v1.Proxy
		snapshot       envoycache.Snapshot
		proxyName      = "proxy-name"
		ref            = "syncer-test"
		ns             = "any-ns"
	)

	BeforeEach(func() {
		var err error
		xdsCache = &MockXdsCache{}
		sanitizer = &MockXdsSanitizer{}
		ctx, cancel = context.WithCancel(context.Background())

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(ctx, resourceClientFactory)

		upstreamClient, err = resourceClientFactory.NewResourceClient(ctx, factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy = &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName,
			},
		}

		settings = &v1.Settings{}

		rep := reporter.NewReporter(ref, proxyClient.BaseClient(), upstreamClient)

		xdsHasher := &xds.ProxyKeyHasher{}

		snapshot = xds.NewEndpointsSnapshotFromResources(
			envoycache.NewResources("current endpoint", []envoycache.Resource{
				resource.NewEnvoyResource(&envoy_config_endpoint_v3.ClusterLoadAssignment{
					ClusterName: "coffee",
				}),
			}),
			envoycache.NewResources("current cluster", []envoycache.Resource{
				resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{
					Name:                 "coffee",
					ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{Type: envoy_config_cluster_v3.Cluster_EDS},
					LbPolicy:             envoy_config_cluster_v3.Cluster_ROUND_ROBIN,
					DnsLookupFamily:      envoy_config_cluster_v3.Cluster_V4_ONLY,
					EdsClusterConfig:     &envoy_config_cluster_v3.Cluster_EdsClusterConfig{},
				}),
			}),
		)
		syncer = NewTranslatorSyncer(&mockTranslator{true, false, snapshot}, xdsCache, xdsHasher, sanitizer, rep, false, nil, settings)

		_, err = proxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		proxies, err := proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))

	})

	AfterEach(func() { cancel() })

	It("only updates endpoints and clusters when sanitization fails and there is no previous snapshot", func() {
		sanitizer.Err = errors.Errorf("we ran out of coffee")

		apiSnap := v1.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy,
			},
		}

		// old snapshot is not set
		xdsCache.GetSnap = nil
		err := syncer.Sync(context.Background(), &apiSnap)
		Expect(err).NotTo(HaveOccurred())

		Expect(sanitizer.Called).To(BeTrue())
		Expect(xdsCache.Called).To(BeTrue())

		// Don't update listener and routes
		newListeners := xdsCache.SetSnap.GetResources(resource.ListenerTypeV3)
		Expect(newListeners.Items).To(BeNil())
		newRoutes := xdsCache.SetSnap.GetResources(resource.RouteTypeV3)
		Expect(newRoutes.Items).To(BeNil())

		// update endpoints and clusters
		newEndpoints := xdsCache.SetSnap.GetResources(resource.EndpointTypeV3)
		Expect(newEndpoints).To(Equal(snapshot.GetResources(resource.EndpointTypeV3)))
		newClusters := xdsCache.SetSnap.GetResources(resource.ClusterTypeV3)
		Expect(newClusters).To(Equal(snapshot.GetResources(resource.ClusterTypeV3)))

		proxies, err := proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].Status).To(Equal(&core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
			ReportedBy: ref,
		}))
	})
})

var _ = Describe("Translate mulitple proxies with errors", func() {

	var (
		xdsCache       *MockXdsCache
		sanitizer      *MockXdsSanitizer
		syncer         v1.ApiSyncer
		snap           *v1.ApiSnapshot
		settings       *v1.Settings
		proxyClient    v1.ProxyClient
		upstreamClient v1.UpstreamClient
		proxyName      = "proxy-name"
		upstreamName   = "upstream-name"
		ref            = "syncer-test"
		ns             = "any-ns"
	)

	proxiesShouldHaveErrors := func(proxies v1.ProxyList, numProxies int) {
		Expect(proxies).To(HaveLen(numProxies))
		for _, proxy := range proxies {
			Expect(proxy).To(BeAssignableToTypeOf(&v1.Proxy{}))
			Expect(proxy.Status).To(Equal(&core.Status{
				State:      2,
				Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
				ReportedBy: ref,
			}))

		}

	}
	writeUniqueErrsToUpstreams := func() {
		// Re-writes existing upstream to have an annotation
		// which triggers a unique error to be written from each proxy's mockTranslator
		upstreams, err := upstreamClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreams).To(HaveLen(1))

		us := upstreams[0]
		// This annotation causes the translator mock to generate a unique error per proxy on each upstream
		us.Metadata.Annotations = map[string]string{"uniqueErrPerProxy": "true"}
		_, err = upstreamClient.Write(us, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
		snap.Upstreams = upstreams
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		var err error
		xdsCache = &MockXdsCache{}
		sanitizer = &MockXdsSanitizer{}

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(context.Background(), resourceClientFactory)

		usClient, err := resourceClientFactory.NewResourceClient(context.Background(), factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy1 := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName + "1",
			},
		}
		proxy2 := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName + "2",
			},
		}

		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      upstreamName,
				Namespace: ns,
			},
		}

		settings = &v1.Settings{}

		rep := reporter.NewReporter(ref, proxyClient.BaseClient(), usClient)

		xdsHasher := &xds.ProxyKeyHasher{}
		syncer = NewTranslatorSyncer(&mockTranslator{true, true, nil}, xdsCache, xdsHasher, sanitizer, rep, false, nil, settings)
		snap = &v1.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy1,
				proxy2,
			},
			Upstreams: v1.UpstreamList{
				us,
			},
		}

		_, err = usClient.Write(us, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		_, err = proxyClient.Write(proxy1, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		_, err = proxyClient.Write(proxy2, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		proxies, err := proxyClient.List(proxy1.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(2))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].Status).To(Equal(&core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
			ReportedBy: ref,
		}))

		// NilSnapshot is always consistent, so snapshot will always be set as part of endpoints update
		Expect(xdsCache.Called).To(BeTrue())

		upstreamClient, err = v1.NewUpstreamClient(context.Background(), resourceClientFactory)
		Expect(err).NotTo(HaveOccurred())
	})

	It("handles reporting errors on multiple proxies sharing an upstream reporting 2 different errors", func() {
		// Testing the scenario where we have multiple proxies,
		// each of which should report a different unique error on an upstream.
		proxies, err := proxyClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		proxiesShouldHaveErrors(proxies, 2)

		writeUniqueErrsToUpstreams()

		upstreams, err := upstreamClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(upstreams[0].Status).To(Equal(&core.Status{
			State:      2,
			Reason:     "2 errors occurred:\n\t* upstream is bad - determined by proxy-name1\n\t* upstream is bad - determined by proxy-name2\n\n",
			ReportedBy: ref,
		}))

		Expect(xdsCache.Called).To(BeTrue())
	})

	It("handles reporting errors on multiple proxies sharing an upstream, each reporting the same upstream error", func() {
		// Testing the scenario where we have multiple proxies,
		// each of which should report the same error on an upstream.
		proxies, err := proxyClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		proxiesShouldHaveErrors(proxies, 2)

		upstreams, err := upstreamClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreams).To(HaveLen(1))
		Expect(upstreams[0].Status).To(Equal(&core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* generic upstream error\n\n",
			ReportedBy: ref,
		}))

		Expect(xdsCache.Called).To(BeTrue())
	})
})

type mockTranslator struct {
	reportErrs         bool
	reportUpstreamErrs bool // Adds an error to every upstream in the snapshot
	currentSnapshot    envoycache.Snapshot
}

func (t *mockTranslator) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceReports, *validation.ProxyReport, error) {
	if t.reportErrs {
		rpts := reporter.ResourceReports{}
		rpts.AddError(proxy, errors.Errorf("hi, how ya doin'?"))
		if t.reportUpstreamErrs {
			for _, upstream := range params.Snapshot.Upstreams {
				if upstream.Metadata.Annotations["uniqueErrPerProxy"] == "true" {
					rpts.AddError(upstream, errors.Errorf("upstream is bad - determined by %s", proxy.Metadata.Name))
				} else {
					rpts.AddError(upstream, errors.Errorf("generic upstream error"))
				}
			}
		}
		if t.currentSnapshot != nil {
			return t.currentSnapshot, rpts, &validation.ProxyReport{}, nil
		}
		return envoycache.NilSnapshot{}, rpts, &validation.ProxyReport{}, nil
	}
	if t.currentSnapshot != nil {
		return t.currentSnapshot, nil, &validation.ProxyReport{}, nil
	}
	return envoycache.NilSnapshot{}, nil, &validation.ProxyReport{}, nil
}

var _ envoycache.SnapshotCache = &MockXdsCache{}
