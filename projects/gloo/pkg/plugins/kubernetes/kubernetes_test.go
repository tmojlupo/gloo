package kubernetes_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubepluginapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	kubecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/client-go/kubernetes/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	kubev1 "k8s.io/api/core/v1"

	"os"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	"k8s.io/client-go/kubernetes"

	// From https://github.com/kubernetes/client-go/blob/53c7adfd0294caa142d961e1f780f74081d5b15f/examples/out-of-cluster-client-configuration/main.go#L31
	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kubernetes", func() {

	BeforeEach(func() {
		if os.Getenv("RUN_KUBE_TESTS") != "1" {
			Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		}
	})

	Context("kubernetes", func() {
		var (
			svcNamespace  string
			svcName       = "i-love-writing-tests"
			kubeClient    kubernetes.Interface
			kubeCoreCache kubecache.KubeCoreCache

			baseLabels = map[string]string{
				"tacos": "burritos",
			}

			extendedLabels = map[string]string{
				"tacos": "burritos",
				"pizza": "frenchfries",
			}

			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctx = settingsutil.WithSettings(ctx, &v1.Settings{})
			svcNamespace = helpers.RandString(8)
			kubeClient = fake.NewSimpleClientset()
			var err error
			kubeCoreCache, err = kubecache.NewKubeCoreCache(context.TODO(), kubeClient)
			Expect(err).NotTo(HaveOccurred())

			// create a service
			// create 2 pods for that service
			// one with extra labels, one without
			svc, err := kubeClient.CoreV1().Services(svcNamespace).Create(ctx, &kubev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: svcNamespace,
					Name:      svcName,
				},
				Spec: kubev1.ServiceSpec{
					Selector: baseLabels,
					Ports: []kubev1.ServicePort{
						{
							Name: "bar",
							Port: 8080,
						},
						{
							Name: "foo",
							Port: 9090,
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			_, err = kubeClient.CoreV1().Pods(svcNamespace).Create(ctx, &kubev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-for-" + svc.Name + "-basic",
					Namespace: svcNamespace,
					Labels:    baseLabels,
				},
				Spec: kubev1.PodSpec{
					Containers: []kubev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			_, err = kubeClient.CoreV1().Pods(svcNamespace).Create(ctx, &kubev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-for-" + svc.Name + "-extra",
					Namespace: svcNamespace,
					Labels:    extendedLabels,
				},
				Spec: kubev1.PodSpec{
					Containers: []kubev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			_, err = kubeClient.CoreV1().Endpoints(svcNamespace).Create(ctx, &kubev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svc.Name,
					Namespace: svcNamespace,
				},
				Subsets: []kubev1.EndpointSubset{{
					Addresses: []kubev1.EndpointAddress{
						{IP: "10.4.0.60"},
						{IP: "10.4.0.61"},
					},
					Ports: []kubev1.EndpointPort{
						{Name: "foo", Port: 9090, Protocol: kubev1.ProtocolTCP},
						{Name: "bar", Port: 8080, Protocol: kubev1.ProtocolTCP},
					},
				}},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			setup.TeardownKube(svcNamespace)
			if cancel != nil {
				cancel()
			}
		})

		It("uses json keys when serializing", func() {
			plug := kubeplugin.NewPlugin(kubeClient, kubeCoreCache).(discovery.DiscoveryPlugin)
			upstreams, errs, err := plug.DiscoverUpstreams([]string{svcNamespace}, svcNamespace, clients.WatchOpts{
				Ctx:         context.TODO(),
				RefreshRate: time.Second,
			}, discovery.Opts{})
			Expect(err).NotTo(HaveOccurred())

			select {
			case <-time.After(time.Second * 2):
				Fail("no upstreams detected after 2s")
			case upstreamList := <-upstreams:
				// two pods, two ports per pod. both pods selected by a single service
				// create an upstream for each port on the service (2)
				Expect(upstreamList).To(HaveLen(2))
				break
			case err, ok := <-errs:
				if !ok {
					return
				}
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("shares endpoints between multiple upstreams that have the same endpoint", func() {
			makeUpstream := func(name string) *v1.Upstream {
				return &v1.Upstream{
					Metadata: &core.Metadata{Name: name},
					UpstreamType: &v1.Upstream_Kube{
						Kube: &kubepluginapi.UpstreamSpec{
							ServiceNamespace: svcNamespace,
							ServiceName:      svcName,
							ServicePort:      8080,
						},
					},
				}
			}
			plug := kubeplugin.NewPlugin(kubeClient, kubeCoreCache).(discovery.DiscoveryPlugin)
			eds, errs, err := plug.WatchEndpoints(
				"",
				v1.UpstreamList{makeUpstream("a"), makeUpstream("b"), makeUpstream("c")},
				clients.WatchOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			Eventually(eds, time.Second).Should(Receive(HaveLen(6)))
			Consistently(errs).Should(Not(Receive()))
		})
	})
})
