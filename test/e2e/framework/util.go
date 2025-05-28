package framework

import (
	"bytes"
	"context"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

type Buffer struct {
	buffer bytes.Buffer
	mutex  sync.Mutex
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(p)
}

func (b *Buffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.String()
}

func NewBuffer() *Buffer {
	return &Buffer{}
}

type Cmd struct {
	*exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
}

func NewCmd(path string, args ...string) *Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, path, args...)
	return &Cmd{
		Cmd:    cmd,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *Cmd) Stop() error {
	c.cancel()
	return c.Wait()
}

type CleanupActionHandle int

var (
	cleanupActionsLock sync.Mutex
	cleanupActions                         = map[CleanupActionHandle]func(){}
	nextCleanupAction  CleanupActionHandle = 0
)

func AddCleanupAction(fn func()) CleanupActionHandle {
	cleanupActionsLock.Lock()
	defer cleanupActionsLock.Unlock()
	handle := nextCleanupAction
	nextCleanupAction++
	cleanupActions[handle] = fn
	return handle
}

func RemoveCleanupAction(handle CleanupActionHandle) {
	cleanupActionsLock.Lock()
	defer cleanupActionsLock.Unlock()
	delete(cleanupActions, handle)
}

func RunCleanupActions() {
	cleanupActionsLock.Lock()
	defer cleanupActionsLock.Unlock()
	for _, fn := range cleanupActions {
		fn()
	}
	cleanupActions = map[CleanupActionHandle]func(){}
}

func Fail(message string, callerSkip ...int) {
	skip := 1
	if len(callerSkip) > 0 {
		skip = callerSkip[0]
	}
	ginkgo.Fail(message, skip)
}

func ExpectNoError(err error, explain ...interface{}) {
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), explain...)
}

func ExpectTrue(actual interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).To(gomega.BeTrue(), explain...)
}

func ExpectEqual(actual, expected interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).To(gomega.Equal(expected), explain...)
}

func AllocPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func ReleasePort(port int) {
	time.Sleep(100 * time.Millisecond) // Small delay to ensure port is fully released
}
