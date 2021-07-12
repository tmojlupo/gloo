package syncer

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	gatewaymocks "github.com/solo-io/gloo/projects/gateway/pkg/translator/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("TranslatorSyncer", func() {
	var (
		fakeWatcher  = &fakeWatcher{}
		mockReporter *fakeReporter
		syncer       *statusSyncer
	)

	BeforeEach(func() {
		mockReporter = &fakeReporter{}
		curSyncer := newStatusSyncer("gloo-system", fakeWatcher, mockReporter)
		syncer = &curSyncer
	})

	getMapOnlyKey := func(r map[string]reporter.Report) string {
		Expect(r).To(HaveLen(1))
		for k := range r {
			return k
		}
		panic("unreachable")
	}

	It("should set status correctly", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		vs := &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "vs",
				Namespace: "gloo-system",
			},
		}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when resources are in both proxies", func() {
		acceptedProxy1 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test1", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		acceptedProxy2 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		errs1 := reporter.ResourceReports{}
		errs2 := reporter.ResourceReports{}
		expectedErr := reporter.ResourceReports{}

		rt := &gatewayv1.RouteTable{
			Metadata: &core.Metadata{
				Name:      "rt",
				Namespace: defaults.GlooSystem,
			},
		}
		errs1.AddWarning(rt, "warning 1")
		errs2.AddWarning(rt, "warning 2")
		expectedErr.AddWarning(rt, "warning 1")
		expectedErr.AddWarning(rt, "warning 2")

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy1: errs1,
			acceptedProxy2: errs2,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy1, acceptedProxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(rt.GetMetadata().Ref())))

		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(expectedErr[rt]))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test1": {State: core.Status_Accepted},
			"*v1.Proxy.gloo-system.test2": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when proxy is pending first", func() {
		desiredProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		pendingProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Pending},
		}
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			desiredProxy: errs,
		}
		proxies := make(chan gloov1.ProxyList)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go syncer.watchProxiesFromChannel(ctx, proxies, nil)
		go syncer.syncStatusOnEmit(ctx)

		syncer.setCurrentProxies(desiredProxies)
		proxies <- gloov1.ProxyList{pendingProxy}
		proxies <- gloov1.ProxyList{acceptedProxy}

		Eventually(mockReporter.Reports, "5s", "0.5s").ShouldNot(BeEmpty())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Eventually(func() map[string]*core.Status { return mockReporter.Statuses()[reportedKey] }, "5s", "0.5s").Should(BeEquivalentTo(m))
	})

	It("should retry setting the status if it first fails", func() {
		desiredProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		mockReporter.Err = fmt.Errorf("error")
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			desiredProxy: errs,
		}
		proxies := make(chan gloov1.ProxyList)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go syncer.watchProxiesFromChannel(ctx, proxies, nil)
		go syncer.syncStatusOnEmit(ctx)

		syncer.setCurrentProxies(desiredProxies)
		proxies <- gloov1.ProxyList{acceptedProxy}

		Eventually(mockReporter.Reports, "5s", "0.5s").ShouldNot(BeEmpty())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Eventually(func() map[string]*core.Status { return mockReporter.Statuses()[reportedKey] }, "5s", "0.5s").Should(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		rejectedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
			rejectedProxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy, rejectedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test":  {State: core.Status_Accepted},
			"*v1.Proxy.gloo-system.test2": {State: core.Status_Rejected},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors but is irrelevant", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Accepted},
		}
		rejectedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
			rejectedProxy: reporter.ResourceReports{},
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy, rejectedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		rejectedProxy1 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Rejected},
		}
		rejectedProxy2 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   &core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errsProxy1 := reporter.ResourceReports{}
		errsProxy1.Accept(vs)
		errsProxy1.AddError(vs, fmt.Errorf("invalid 1"))
		errsProxy2 := reporter.ResourceReports{}
		errsProxy2.Accept(vs)
		errsProxy2.AddError(vs, fmt.Errorf("invalid 2"))
		desiredProxies := reconciler.GeneratedProxies{
			rejectedProxy1: errsProxy1,
			rejectedProxy2: errsProxy2,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{rejectedProxy1, rejectedProxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		mergedErrs := reporter.ResourceReports{}
		mergedErrs.Accept(vs)
		mergedErrs.AddError(vs, fmt.Errorf("invalid 1"))
		mergedErrs.AddError(vs, fmt.Errorf("invalid 2"))

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(mergedErrs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test":  {State: core.Status_Rejected},
			"*v1.Proxy.gloo-system.test2": {State: core.Status_Rejected},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	Context("translator syncer", func() {
		var (
			mockTranslator *gatewaymocks.MockTranslator
			ctrl           *gomock.Controller

			ctx      context.Context
			settings *gloov1.Settings

			ts    *translatorSyncer
			snap  *gatewayv1.ApiSnapshot
			proxy *gloov1.Proxy
		)
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			mockTranslator = gatewaymocks.NewMockTranslator(ctrl)
			settings = &gloov1.Settings{
				Gateway: &gloov1.GatewayOptions{
					CompressedProxySpec: true,
				},
			}
			ctx = context.Background()

			ts = &translatorSyncer{
				writeNamespace: "gloo-system",
				translator:     mockTranslator,
			}
			snap = &gatewayv1.ApiSnapshot{
				Gateways: gatewayv1.GatewayList{
					&gatewayv1.Gateway{},
				},
			}
			proxy = &gloov1.Proxy{
				Metadata: &core.Metadata{
					Name: "proxy",
				},
			}
		})
		AfterEach(func() {
			ctrl.Finish()
		})

		It("should compress proxy spec when setttings are set", func() {

			ctx = settingsutil.WithSettings(ctx, settings)

			mockTranslator.EXPECT().Translate(gomock.Any(), "gateway-proxy", "gloo-system", snap, gomock.Any()).
				Return(proxy, nil)

			ts.generatedDesiredProxies(ctx, snap)

			Expect(proxy.Metadata.Annotations).To(HaveKeyWithValue(compress.CompressedKey, compress.CompressedValue))
		})

		It("should not compress proxy spec when setttings are not set", func() {
			mockTranslator.EXPECT().Translate(gomock.Any(), "gateway-proxy", "gloo-system", snap, gomock.Any()).
				Return(proxy, nil)

			ts.generatedDesiredProxies(ctx, snap)

			Expect(proxy.Metadata.Annotations).NotTo(HaveKeyWithValue(compress.CompressedKey, compress.CompressedValue))
		})

	})

})

type fakeWatcher struct {
}

func (f *fakeWatcher) Watch(namespace string, opts clients.WatchOpts) (<-chan gloov1.ProxyList, <-chan error, error) {
	return nil, nil, nil
}

type fakeReporter struct {
	reports  map[string]reporter.Report
	statuses map[string]map[string]*core.Status
	lock     sync.Mutex
	Err      error
}

func (f *fakeReporter) Reports() map[string]reporter.Report {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.reports
}
func (f *fakeReporter) Statuses() map[string]map[string]*core.Status {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.statuses
}

func (f *fakeReporter) WriteReports(ctx context.Context, errs reporter.ResourceReports, subresourceStatuses map[string]*core.Status) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	fmt.Fprintf(GinkgoWriter, "WriteReports: %#v %#v", errs, subresourceStatuses)
	newreports := map[string]reporter.Report{}
	for k, v := range f.reports {
		newreports[k] = v
	}
	for k, v := range errs {
		newreports[translator.UpstreamToClusterName(k.GetMetadata().Ref())] = v
	}
	f.reports = newreports

	newstatus := map[string]map[string]*core.Status{}
	for k, v := range f.statuses {
		newstatus[k] = v
	}
	for k := range errs {
		newstatus[translator.UpstreamToClusterName(k.GetMetadata().Ref())] = subresourceStatuses
	}
	f.statuses = newstatus

	err := f.Err
	f.Err = nil
	return err
}

func (f *fakeReporter) StatusFromReport(report reporter.Report, subresourceStatuses map[string]*core.Status) *core.Status {
	return &core.Status{}
}
