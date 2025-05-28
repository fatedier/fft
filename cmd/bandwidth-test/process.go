package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type Process struct {
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	errorOutput *bytes.Buffer
	stdOutput   *bytes.Buffer
	mutex       sync.Mutex
	stopped     bool
	name        string // Process name for logging
	showOutput  bool   // Whether to show output in real-time
}

type OutputWriter struct {
	buffer *bytes.Buffer
	prefix string
}

func (w *OutputWriter) Write(p []byte) (n int, err error) {
	n, err = w.buffer.Write(p)
	if err != nil {
		return n, err
	}
	
	fmt.Print(w.prefix)
	fmt.Print(string(p))
	
	return n, nil
}

func NewProcess(name, path string, params []string, showOutput bool) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, path, params...)
	p := &Process{
		cmd:        cmd,
		cancel:     cancel,
		name:       name,
		showOutput: showOutput,
	}
	p.errorOutput = bytes.NewBufferString("")
	p.stdOutput = bytes.NewBufferString("")
	
	if showOutput {
		stdoutWriter := &OutputWriter{
			buffer: p.stdOutput,
			prefix: fmt.Sprintf("[%s] ", name),
		}
		stderrWriter := &OutputWriter{
			buffer: p.errorOutput,
			prefix: fmt.Sprintf("[%s ERROR] ", name),
		}
		cmd.Stdout = stdoutWriter
		cmd.Stderr = stderrWriter
	} else {
		cmd.Stdout = p.stdOutput
		cmd.Stderr = p.errorOutput
	}
	
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

func GetExecutablePath(name string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Sprintf("./bin/%s", name)
	}
	
	return filepath.Join(cwd, "bin", name)
}
