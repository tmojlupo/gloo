package ingress_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/go-utils/log"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/go-utils/testutils/helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestIngress(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "ingress" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'ingress' in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Ingress Suite")
}

var testHelper *helper.SoloTestHelper

var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	randomNumber := time.Now().Unix() % 10000
	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "ingress-test-" + fmt.Sprintf("%d-%d", randomNumber, GinkgoParallelNode())
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))
	testHelper.Verbose = true

	// Install Gloo
	err = testHelper.InstallGloo(helper.INGRESS, 5*time.Minute)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if os.Getenv("TEAR_DOWN") == "true" {
		err := testHelper.UninstallGlooAll()
		Expect(err).NotTo(HaveOccurred())

		// TODO go-utils should expose `glooctl uninstall --delete-namespace`
		testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

		Eventually(func() error {
			return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
		}, "60s", "1s").Should(HaveOccurred())
	}
})
