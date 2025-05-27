package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/onsi/ginkgo/v2"
)

var (
	TestContext struct {
		FFTPath    string
		FFTSPath   string
		FFTWPath   string
		LogLevel   string
		Debug      bool
	}

	RunID = fmt.Sprintf("%d", ginkgo.GinkgoRandomSeed())
)

type Framework struct {
	TempDirectory string

	usedPorts map[string]int

	allocatedPorts []int

	cleanupHandle CleanupActionHandle

	beforeEachStarted bool

	serverConfPath string
	serverProcess *Process

	workerConfPaths []string
	workerProcesses []*Process

	clientConfPaths []string
	clientProcesses []*Process

	configFileIndex int64

	osEnvs []string

	mutex sync.Mutex
}

func NewDefaultFramework() *Framework {
	f := &Framework{
		usedPorts: make(map[string]int),
	}

	ginkgo.BeforeEach(f.BeforeEach)
	ginkgo.AfterEach(f.AfterEach)
	return f
}

func (f *Framework) BeforeEach() {
	f.beforeEachStarted = true

	f.cleanupHandle = AddCleanupAction(f.AfterEach)

	dir, err := os.MkdirTemp(os.TempDir(), "fft-e2e-test-*")
	ExpectNoError(err)
	f.TempDirectory = dir
}

func (f *Framework) AfterEach() {
	if !f.beforeEachStarted {
		return
	}

	RemoveCleanupAction(f.cleanupHandle)

	if f.serverProcess != nil {
		_ = f.serverProcess.Stop()
		if TestContext.Debug || ginkgo.CurrentSpecReport().Failed() {
			fmt.Println(f.serverProcess.ErrorOutput())
			fmt.Println(f.serverProcess.StdOutput())
		}
	}
	for _, p := range f.workerProcesses {
		_ = p.Stop()
		if TestContext.Debug || ginkgo.CurrentSpecReport().Failed() {
			fmt.Println(p.ErrorOutput())
			fmt.Println(p.StdOutput())
		}
	}
	for _, p := range f.clientProcesses {
		_ = p.Stop()
		if TestContext.Debug || ginkgo.CurrentSpecReport().Failed() {
			fmt.Println(p.ErrorOutput())
			fmt.Println(p.StdOutput())
		}
	}
	f.serverProcess = nil
	f.workerProcesses = nil
	f.clientProcesses = nil

	os.RemoveAll(f.TempDirectory)
	f.TempDirectory = ""
	f.serverConfPath = ""
	f.workerConfPaths = []string{}
	f.clientConfPaths = []string{}

	for _, port := range f.usedPorts {
		ReleasePort(port)
	}
	f.usedPorts = make(map[string]int)

	for _, port := range f.allocatedPorts {
		ReleasePort(port)
	}
	f.allocatedPorts = make([]int, 0)

	f.osEnvs = make([]string, 0)
}

func (f *Framework) AllocPort() int {
	port := AllocPort()
	ExpectTrue(port > 0, "alloc port failed")
	f.allocatedPorts = append(f.allocatedPorts, port)
	return port
}

func (f *Framework) PortByName(name string) int {
	return f.usedPorts[name]
}

func (f *Framework) SetEnvs(envs []string) {
	f.osEnvs = envs
}

func (f *Framework) WriteTempFile(name string, content string) string {
	filePath := filepath.Join(f.TempDirectory, name)
	err := os.WriteFile(filePath, []byte(content), 0o600)
	ExpectNoError(err)
	return filePath
}

func (f *Framework) GenerateConfigFile(content string) string {
	f.configFileIndex++
	path := filepath.Join(f.TempDirectory, fmt.Sprintf("fft-e2e-config-%d", f.configFileIndex))
	err := os.WriteFile(path, []byte(content), 0o600)
	ExpectNoError(err)
	return path
}
