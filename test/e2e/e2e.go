package e2e

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/fatedier/fft/test/e2e/framework"
)

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	setupSuite()
	return nil
}, func(data []byte) {
	setupSuitePerGinkgoNode()
})

var _ = ginkgo.SynchronizedAfterSuite(func() {
	CleanupSuite()
}, func() {
	AfterSuiteActions()
})

func RunE2ETests(t *testing.T) {
	gomega.RegisterFailHandler(framework.Fail)

	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	suiteConfig.EmitSpecProgress = true
	suiteConfig.RandomizeAllSpecs = true

	ginkgo.RunSpecs(t, "fft e2e suite", suiteConfig, reporterConfig)
}

//
func setupSuite() {
}

func setupSuitePerGinkgoNode() {
}

func CleanupSuite() {
	framework.RunCleanupActions()
}

func AfterSuiteActions() {
}
