package process

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
)

type Process struct {
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	errorOutput *bytes.Buffer
	stdOutput   *bytes.Buffer
	mutex       sync.Mutex
	stopped     bool
}

func New(path string, params []string) *Process {
	return NewWithEnvs(path, params, nil)
}

func NewWithEnvs(path string, params []string, envs []string) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, path, params...)
	cmd.Env = envs
	p := &Process{
		cmd:    cmd,
		cancel: cancel,
	}
	p.errorOutput = bytes.NewBufferString("")
	p.stdOutput = bytes.NewBufferString("")
	cmd.Stderr = p.errorOutput
	cmd.Stdout = p.stdOutput
	return p
}

func (p *Process) Start() error {
	return p.cmd.Start()
}

func (p *Process) Stop() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.stopped {
		return nil
	}

	p.stopped = true
	p.cancel()
	return p.cmd.Wait()
}

func (p *Process) ErrorOutput() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.errorOutput.String()
}

func (p *Process) StdOutput() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.stdOutput.String()
}
