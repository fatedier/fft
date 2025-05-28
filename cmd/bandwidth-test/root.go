package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/msg"
	"github.com/fatedier/fft/pkg/sender"
	"github.com/fatedier/fft/pkg/stream"
	"github.com/fatedier/fft/server"
	"github.com/fatedier/fft/version"
	"github.com/fatedier/fft/worker"

	"github.com/cheggaaa/pb"
	"github.com/spf13/cobra"
)

var (
	showVersion bool
	fileSize    int64
	duration    int
	tempDir     string
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of bandwidth test tool")
	rootCmd.PersistentFlags().Int64VarP(&fileSize, "file_size", "s", 0, "test file size in bytes, 0 means auto calculate based on duration")
	rootCmd.PersistentFlags().IntVarP(&duration, "duration", "d", 25, "expected test duration in seconds, used to calculate file size if not specified")
	rootCmd.PersistentFlags().StringVarP(&tempDir, "temp_dir", "t", os.TempDir(), "directory to store temporary files")
}

var rootCmd = &cobra.Command{
	Use:   "bandwidth-test",
	Short: "bandwidth-test is a tool to test bandwidth aggregation in fft (https://github.com/fatedier/fft)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(version.Full())
			return nil
		}

		return runBandwidthTest()
	},
}

func runBandwidthTest() error {
	fmt.Println("Starting bandwidth aggregation test...")

	testDir := filepath.Join(tempDir, fmt.Sprintf("fft-bandwidth-test-%d", time.Now().UnixNano()))
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	serverPort, err := allocPort()
	if err != nil {
		return fmt.Errorf("failed to allocate server port: %v", err)
	}
	serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)

	serverOptions := server.Options{
		BindAddr: serverAddr,
	}
	serverSvc, err := server.NewService(serverOptions)
	if err != nil {
		return fmt.Errorf("failed to create server service: %v", err)
	}

	go func() {
		if err := serverSvc.Run(); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	}()
	fmt.Printf("Server started on %s\n", serverAddr)
	time.Sleep(1 * time.Second)

	worker1Port, err := allocPort()
	if err != nil {
		return fmt.Errorf("failed to allocate worker1 port: %v", err)
	}
	worker1Addr := fmt.Sprintf("127.0.0.1:%d", worker1Port)

	worker1Options := worker.Options{
		ServerAddr:     serverAddr,
		BindAddr:       worker1Addr,
		AdvicePublicIP: "127.0.0.1",
		RateKB:         100, // 100KB/s
	}
	worker1Svc, err := worker.NewService(worker1Options)
	if err != nil {
		return fmt.Errorf("failed to create worker1 service: %v", err)
	}

	go func() {
		if err := worker1Svc.Run(); err != nil {
			fmt.Printf("Worker1 error: %v\n", err)
		}
	}()
	fmt.Printf("Worker1 started on %s with bandwidth limit 100KB/s\n", worker1Addr)

	worker2Port, err := allocPort()
	if err != nil {
		return fmt.Errorf("failed to allocate worker2 port: %v", err)
	}
	worker2Addr := fmt.Sprintf("127.0.0.1:%d", worker2Port)

	worker2Options := worker.Options{
		ServerAddr:     serverAddr,
		BindAddr:       worker2Addr,
		AdvicePublicIP: "127.0.0.1",
		RateKB:         500, // 500KB/s
	}
	worker2Svc, err := worker.NewService(worker2Options)
	if err != nil {
		return fmt.Errorf("failed to create worker2 service: %v", err)
	}

	go func() {
		if err := worker2Svc.Run(); err != nil {
			fmt.Printf("Worker2 error: %v\n", err)
		}
	}()
	fmt.Printf("Worker2 started on %s with bandwidth limit 500KB/s\n", worker2Addr)

	time.Sleep(2 * time.Second)

	if fileSize == 0 {
		expectedSpeed := 600 * 1024 * 0.4 // 240KB/s in bytes/sec
		fileSize = int64(expectedSpeed * float64(duration))
		fmt.Printf("Auto-calculated file size: %d bytes (%.2f MB) for %d seconds test\n",
			fileSize, float64(fileSize)/(1024*1024), duration)
	}

	testFilePath := filepath.Join(testDir, "test-file")
	err = createTestFile(testFilePath, fileSize)
	if err != nil {
		return fmt.Errorf("failed to create test file: %v", err)
	}

	recvDir := filepath.Join(testDir, "recv")
	err = os.MkdirAll(recvDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create receive directory: %v", err)
	}

	transferID := fmt.Sprintf("bandwidth-test-%d", time.Now().UnixNano())

	recvBar := pb.New(int(fileSize))
	recvBar.ShowSpeed = true
	recvBar.SetUnits(pb.U_BYTES)
	recvBar.Start()

	var receivedBytes int64
	var recvMu sync.Mutex
	recvCallback := func(n int) {
		recvMu.Lock()
		receivedBytes += int64(n)
		recvMu.Unlock()
		recvBar.Add(n)
	}

	fmt.Println("Starting file transfer...")
	startTime := time.Now()

	var totalBytes int64
	var mu sync.Mutex

	finfo, err := os.Stat(testFilePath)
	if err != nil {
		return fmt.Errorf("failed to stat test file: %v", err)
	}

	bar := pb.New(int(finfo.Size()))
	bar.ShowSpeed = true
	bar.SetUnits(pb.U_BYTES)
	bar.Start()

	callback := func(n int) {
		mu.Lock()
		totalBytes += int64(n)
		mu.Unlock()
		bar.Add(n)
	}

	senderDoneCh := make(chan error, 1)
	receiverDoneCh := make(chan error, 1)

	fmt.Println("Starting sender...")
	go func() {
		err := sendFile(serverAddr, transferID, testFilePath, 5*1024, 512, callback)
		senderDoneCh <- err
	}()

	time.Sleep(2 * time.Second)

	fmt.Println("Starting receiver...")
	go func() {
		err := recvFile(serverAddr, transferID, recvDir, 512, recvCallback)
		receiverDoneCh <- err
	}()

	senderErr := <-senderDoneCh
	if senderErr != nil {
		return fmt.Errorf("sender error: %v", senderErr)
	}

	select {
	case receiverErr := <-receiverDoneCh:
		if receiverErr != nil {
			return fmt.Errorf("receiver error: %v", receiverErr)
		}
	case <-time.After(5 * time.Second):
		fmt.Println("Receiver timeout - this is expected as sender has completed")
	}

	bar.Finish()

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	bytesPerSecond := float64(totalBytes) / duration.Seconds()
	kbPerSecond := bytesPerSecond / 1024

	fmt.Printf("\nBandwidth Test Results:\n")
	fmt.Printf("Total bytes transferred: %d (%.2f MB)\n", totalBytes, float64(totalBytes)/(1024*1024))
	fmt.Printf("Transfer duration: %.2f seconds\n", duration.Seconds())
	fmt.Printf("Average transfer speed: %.2f KB/s\n", kbPerSecond)
	fmt.Printf("Expected combined speed: 600 KB/s (100 KB/s + 500 KB/s)\n")
	fmt.Printf("Efficiency: %.2f%%\n", (kbPerSecond/600)*100)

	receivedFilePath := filepath.Join(recvDir, filepath.Base(testFilePath))
	receivedInfo, err := os.Stat(receivedFilePath)
	if err != nil {
		return fmt.Errorf("failed to stat received file: %v", err)
	}

	if receivedInfo.Size() != fileSize {
		return fmt.Errorf("received file size mismatch: got %d, expected %d", receivedInfo.Size(), fileSize)
	}

	fmt.Println("File transfer completed successfully!")

	return nil
}

func allocPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

func createTestFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	const bufSize = 64 * 1024
	buf := make([]byte, bufSize)

	rand.Read(buf)

	remaining := size
	for remaining > 0 {
		writeSize := bufSize
		if remaining < bufSize {
			writeSize = int(remaining)
		}

		n, err := f.Write(buf[:writeSize])
		if err != nil {
			return err
		}

		remaining -= int64(n)
	}

	return nil
}

func createSender(src io.Reader, frameSize int, cacheCount int) (*sender.Sender, error) {
	return sender.NewSender(0, src, frameSize, cacheCount)
}

func connectToWorker(s *sender.Sender, id string, addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("[%s] Error connecting to worker: %v\n", addr, err)
		return
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	defer conn.Close()

	msg.WriteMsg(conn, &msg.NewSendFileStream{
		ID: id,
	})

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		fmt.Printf("[%s] Error reading response: %v\n", addr, err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	m, ok := raw.(*msg.NewSendFileStreamResp)
	if !ok {
		fmt.Printf("[%s] Invalid response format\n", addr)
		return
	}

	if m.Error != "" {
		fmt.Printf("[%s] Worker error: %s\n", addr, m.Error)
		return
	}

	s.HandleStream(stream.NewFrameStream(conn))
}
