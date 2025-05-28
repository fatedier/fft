package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	workers     string
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of bandwidth test tool")
	rootCmd.PersistentFlags().Int64VarP(&fileSize, "file_size", "s", 0, "test file size in bytes, 0 means auto calculate based on duration")
	rootCmd.PersistentFlags().IntVarP(&duration, "duration", "d", 25, "expected test duration in seconds, used to calculate file size if not specified")
	rootCmd.PersistentFlags().StringVarP(&tempDir, "temp_dir", "t", os.TempDir(), "directory to store temporary files")
	rootCmd.PersistentFlags().StringVarP(&workers, "workers", "w", "100KB,500KB", "worker bandwidth configuration, comma-separated list of bandwidth limits (e.g., '200KB' for one worker, '200KB,200KB,300KB' for three workers)")
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

func parseWorkerBandwidths(workersStr string) ([]int, error) {
	if workersStr == "" {
		return []int{100, 500}, nil // Default: 100KB/s and 500KB/s
	}

	parts := strings.Split(workersStr, ",")
	rates := make([]int, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)

		part = strings.TrimSuffix(part, "KB")

		rate, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid bandwidth format '%s': %v", part, err)
		}

		if rate < 50 {
			return nil, fmt.Errorf("bandwidth must be at least 50KB/s, got %dKB/s", rate)
		}

		rates = append(rates, rate)
	}

	if len(rates) == 0 {
		return nil, fmt.Errorf("no valid worker bandwidths specified")
	}

	return rates, nil
}

func runBandwidthTest() error {
	fmt.Println("Starting bandwidth aggregation test...")

	testDir := filepath.Join(tempDir, fmt.Sprintf("fft-bandwidth-test-%d", time.Now().UnixNano()))
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	workerRates, err := parseWorkerBandwidths(workers)
	if err != nil {
		return fmt.Errorf("failed to parse worker configuration: %v", err)
	}

	var totalBandwidth int
	for _, rate := range workerRates {
		totalBandwidth += rate
	}

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

	workerServices := make([]*worker.Service, 0, len(workerRates))
	workerAddresses := make([]string, 0, len(workerRates))

	for i, rate := range workerRates {
		workerPort, err := allocPort()
		if err != nil {
			return fmt.Errorf("failed to allocate worker%d port: %v", i+1, err)
		}
		workerAddr := fmt.Sprintf("127.0.0.1:%d", workerPort)
		workerAddresses = append(workerAddresses, workerAddr)

		workerOptions := worker.Options{
			ServerAddr:     serverAddr,
			BindAddr:       workerAddr,
			AdvicePublicIP: "127.0.0.1",
			RateKB:         rate,
		}

		workerSvc, err := worker.NewService(workerOptions)
		if err != nil {
			return fmt.Errorf("failed to create worker%d service: %v", i+1, err)
		}
		workerServices = append(workerServices, workerSvc)

		workerIndex := i + 1
		go func(idx int, svc *worker.Service, addr string, bw int) {
			if err := svc.Run(); err != nil {
				fmt.Printf("Worker%d error: %v\n", idx, err)
			}
		}(workerIndex, workerSvc, workerAddr, rate)

		fmt.Printf("Worker%d started on %s with bandwidth limit %dKB/s\n", workerIndex, workerAddr, rate)
	}

	time.Sleep(2 * time.Second)

	if fileSize == 0 {
		expectedSpeed := float64(totalBandwidth) * 1024 * 0.4 // KB/s to bytes/sec with efficiency factor
		fileSize = int64(expectedSpeed * float64(duration))
		fmt.Printf("Auto-calculated file size: %d bytes (%.2f MB) for %d seconds test\n",
			fileSize, float64(fileSize)/(1024*1024), duration)
		fmt.Printf("Based on total bandwidth of %dKB/s across %d workers\n",
			totalBandwidth, len(workerRates))
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

	fmt.Printf("Worker configuration: ")
	for i, rate := range workerRates {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%dKB/s", rate)
	}
	fmt.Printf("\n")

	fmt.Printf("Expected combined speed: %d KB/s\n", totalBandwidth)
	fmt.Printf("Efficiency: %.2f%%\n", (kbPerSecond/float64(totalBandwidth))*100)

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
