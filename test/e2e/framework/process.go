package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (f *Framework) RunServer(args ...string) (*Process, string, error) {
	p := NewWithEnvs(TestContext.FFTSPath, args, f.osEnvs)
	f.serverProcess = p
	err := p.Start()
	if err != nil {
		return p, p.StdOutput(), err
	}
	time.Sleep(2 * time.Second)
	return p, p.StdOutput(), nil
}

func (f *Framework) RunWorker(args ...string) (*Process, string, error) {
	p := NewWithEnvs(TestContext.FFTWPath, args, f.osEnvs)
	f.workerProcesses = append(f.workerProcesses, p)
	err := p.Start()
	if err != nil {
		return p, p.StdOutput(), err
	}
	time.Sleep(2 * time.Second)
	return p, p.StdOutput(), nil
}

func (f *Framework) RunClient(args ...string) (*Process, string, error) {
	p := NewWithEnvs(TestContext.FFTPath, args, f.osEnvs)
	f.clientProcesses = append(f.clientProcesses, p)
	err := p.Start()
	if err != nil {
		return p, p.StdOutput(), err
	}
	time.Sleep(1 * time.Second)
	return p, p.StdOutput(), nil
}

type Process struct {
	cmd         *Cmd
	errorOutput *Buffer
	stdOutput   *Buffer
	stopped     bool
}

func NewWithEnvs(path string, params []string, envs []string) *Process {
	cmd := NewCmd(path, params...)
	cmd.Env = envs
	p := &Process{
		cmd: cmd,
	}
	p.errorOutput = NewBuffer()
	p.stdOutput = NewBuffer()
	cmd.Stderr = p.errorOutput
	cmd.Stdout = p.stdOutput
	return p
}

func (p *Process) Start() error {
	return p.cmd.Start()
}

func (p *Process) Stop() error {
	if p.stopped {
		return nil
	}
	defer func() {
		p.stopped = true
	}()
	return p.cmd.Stop()
}

func (p *Process) ErrorOutput() string {
	return p.errorOutput.String()
}

func (p *Process) StdOutput() string {
	return p.stdOutput.String()
}

func (f *Framework) CreateTestFile(content []byte) (string, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	filename := fmt.Sprintf("test-file-%d", time.Now().UnixNano())
	filepath := filepath.Join(f.TempDirectory, filename)
	
	err := os.WriteFile(filepath, content, 0644)
	if err != nil {
		return "", err
	}
	
	return filepath, nil
}

func (f *Framework) VerifyFileContent(path string, expected []byte) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	if len(content) != len(expected) {
		return fmt.Errorf("file content length mismatch: got %d, expected %d", len(content), len(expected))
	}
	
	for i := range content {
		if content[i] != expected[i] {
			return fmt.Errorf("file content mismatch at position %d: got %d, expected %d", i, content[i], expected[i])
		}
	}
	
	return nil
}
