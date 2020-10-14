package install_test

import (
	"bytes"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install/mocks"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Install", func() {
	var (
		mockHelmClient       *mocks.MockHelmClient
		mockHelmInstallation *mocks.MockHelmInstallation
		ctrl                 *gomock.Controller
		chart                *helmchart.Chart
		helmRelease          *release.Release

		glooOsVersion          = "test"
		glooOsChartUri         = "https://storage.googleapis.com/solo-public-helm/charts/gloo-test.tgz"
		glooEnterpriseChartUri = "https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-test.tgz"
		glooFederationChartUri = "https://storage.googleapis.com/gloo-fed-helm/gloo-fed-test.tgz"
		testCrdContent         = "test-crd-content"
		testHookContent        = `
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: gloo-gateway-secret-create-vwc-update-gloo-system
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "5" # must be executed before cert-gen job
subjects:
- kind: ServiceAccount
  name: gateway-certgen
  namespace: gloo-system
roleRef:
  kind: ClusterRole
  name: gloo-gateway-secret-create-vwc-update-gloo-system
  apiGroup: rbac.authorization.k8s.io
`
		testCleanupHook = `
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: gloo-gateway-secret-create-vwc-update-gloo-system
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": post-install
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "get", "update"]
- apiGroups: ["admissionregistration.k8s.io"]
  resources: ["validatingwebhookconfigurations"]
  verbs: ["get", "update"]
`
	)

	BeforeEach(func() {
		version.Version = glooOsVersion

		ctrl = gomock.NewController(GinkgoT())
		mockHelmClient = mocks.NewMockHelmClient(ctrl)
		mockHelmInstallation = mocks.NewMockHelmInstallation(ctrl)

		chart = &helmchart.Chart{
			Metadata: &helmchart.Metadata{
				Name: "gloo-installer-test-chart",
			},
			Files: []*helmchart.File{{
				Name: "crds/crdA.yaml",
				Data: []byte(testCrdContent),
			}},
		}
		helmRelease = &release.Release{
			Chart: chart,
			Hooks: []*release.Hook{
				{
					Manifest: testHookContent,
				},
				{
					Manifest: testCleanupHook,
				},
			},
			Namespace: defaults.GlooSystem,
		}
	})

	AfterEach(func() {
		version.Version = version.UndefinedVersion
		ctrl.Finish()
	})

	installWithConfig := func(mode install.Mode, expectedValues map[string]interface{}, expectedChartUri string, installConfig *options.Install) {

		helmEnv := &cli.EnvSettings{
			KubeConfig: "path-to-kube-config",
		}

		mockHelmInstallation.EXPECT().
			Run(chart, expectedValues).
			Return(helmRelease, nil)

		mockHelmClient.EXPECT().
			NewInstall(installConfig.Namespace, installConfig.HelmReleaseName, installConfig.DryRun).
			Return(mockHelmInstallation, helmEnv, nil)

		mockHelmClient.EXPECT().
			DownloadChart(expectedChartUri).
			Return(chart, nil)

		mockHelmClient.EXPECT().
			ReleaseExists(installConfig.Namespace, installConfig.HelmReleaseName).
			Return(false, nil)

		dryRunOutputBuffer := new(bytes.Buffer)

		kubeNsClient := fake.NewSimpleClientset().CoreV1().Namespaces()
		installer := install.NewInstallerWithWriter(mockHelmClient, kubeNsClient, dryRunOutputBuffer)
		err := installer.Install(&install.InstallerConfig{
			InstallCliArgs: installConfig,
			Mode:           mode,
		})
		Expect(err).NotTo(HaveOccurred(), "No error should result from the installation")
		Expect(dryRunOutputBuffer.String()).To(BeEmpty())

		// Check that namespace was created
		_, err = kubeNsClient.Get(installConfig.Namespace, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	defaultInstall := func(mode install.Mode, expectedValues map[string]interface{}, expectedChartUri string) {
		installConfig := &options.Install{
			HelmInstall: options.HelmInstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				Version:         "test",
				CreateNamespace: true,
			},
		}
		if mode == install.Federation {
			installConfig.Namespace = defaults.GlooFed
			installConfig.HelmReleaseName = constants.GlooFedReleaseName
		}

		installWithConfig(mode, expectedValues, expectedChartUri, installConfig)
	}

	It("installs cleanly by default", func() {
		defaultInstall(install.Gloo,
			map[string]interface{}{
				"crds": map[string]interface{}{
					"create": false,
				},
			},
			glooOsChartUri)
	})

	It("installs enterprise cleanly by default", func() {

		chart.AddDependency(&helmchart.Chart{Metadata: &helmchart.Metadata{Name: constants.GlooReleaseName}})
		defaultInstall(install.Enterprise,
			map[string]interface{}{
				"gloo": map[string]interface{}{
					"crds": map[string]interface{}{
						"create": false,
					},
				},
			},
			glooEnterpriseChartUri)
	})

	It("installs federation cleanly by default", func() {

		defaultInstall(install.Federation,
			map[string]interface{}{},
			glooFederationChartUri)
	})

	It("installs as enterprise cleanly if passed enterprise helmchart override", func() {

		installConfig := &options.Install{
			HelmInstall: options.HelmInstall{
				Namespace:         defaults.GlooSystem,
				HelmReleaseName:   constants.GlooReleaseName,
				CreateNamespace:   true,
				HelmChartOverride: glooEnterpriseChartUri,
			},
		}

		chart.AddDependency(&helmchart.Chart{Metadata: &helmchart.Metadata{Name: constants.GlooReleaseName}})
		installWithConfig(install.Gloo,
			map[string]interface{}{
				"gloo": map[string]interface{}{
					"crds": map[string]interface{}{
						"create": false,
					},
				},
			},
			glooEnterpriseChartUri,
			installConfig)
	})

	It("installs as open-source cleanly if passed open-source helmchart override with enterprise subcommand", func() {

		installConfig := &options.Install{
			HelmInstall: options.HelmInstall{
				Namespace:         defaults.GlooSystem,
				HelmReleaseName:   constants.GlooReleaseName,
				CreateNamespace:   true,
				HelmChartOverride: glooOsChartUri,
			},
		}

		installWithConfig(install.Gloo,
			map[string]interface{}{
				"crds": map[string]interface{}{
					"create": false,
				},
			},
			glooOsChartUri,
			installConfig)
	})

	It("outputs the expected kinds when in a dry run", func() {
		installConfig := &options.Install{
			HelmInstall: options.HelmInstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DryRun:          true,
				Version:         glooOsVersion,
			},
		}

		helmEnv := &cli.EnvSettings{
			KubeConfig: "path-to-kube-config",
		}

		mockHelmInstallation.EXPECT().
			Run(chart, map[string]interface{}{
				"crds": map[string]interface{}{
					"create": false,
				},
			}).
			Return(helmRelease, nil)

		mockHelmClient.EXPECT().
			NewInstall(defaults.GlooSystem, installConfig.HelmReleaseName, installConfig.DryRun).
			Return(mockHelmInstallation, helmEnv, nil)

		mockHelmClient.EXPECT().
			DownloadChart(glooOsChartUri).
			Return(chart, nil)

		kubeNsClient := fake.NewSimpleClientset().CoreV1().Namespaces()
		dryRunOutputBuffer := new(bytes.Buffer)
		installer := install.NewInstallerWithWriter(mockHelmClient, kubeNsClient, dryRunOutputBuffer)

		err := installer.Install(&install.InstallerConfig{
			InstallCliArgs: installConfig,
		})

		Expect(err).NotTo(HaveOccurred(), "No error should result from the installation")

		dryRunOutput := dryRunOutputBuffer.String()

		Expect(dryRunOutput).To(ContainSubstring(testCrdContent), "Should output CRD definitions")
		Expect(dryRunOutput).To(ContainSubstring("helm.sh/hook"), "Should output non-cleanup hooks")

		// Make sure that namespace was not created
		_, err = kubeNsClient.Get(installConfig.Namespace, metav1.GetOptions{})
		Expect(err).To(HaveOccurred())
	})
})
