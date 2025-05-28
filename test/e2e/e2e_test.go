package e2e

import (
	"flag"
	"fmt"
	"os"
	"testing"

	_ "github.com/onsi/ginkgo/v2"

	_ "github.com/fatedier/fft/test/e2e/basic"
	"github.com/fatedier/fft/test/e2e/framework"
)

func handleFlags() {
	framework.RegisterCommonFlags(flag.CommandLine)
	flag.Parse()
}

func TestMain(m *testing.M) {
	handleFlags()

	if err := framework.ValidateTestContext(&framework.TestContext); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {
	RunE2ETests(t)
}
