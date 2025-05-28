package main

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatedier/fft/version"

	"github.com/cheggaaa/pb"
	"github.com/spf13/cobra"
)

var (
	showVersion bool
	fileSize    int64
	duration    int
	tempDir     string
	workers     string
	verbose     bool // Whether to show detailed output from external processes
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of bandwidth test tool")
	rootCmd.PersistentFlags().Int64VarP(&fileSize, "file-size", "s", 0, "test file size in KB, 0 means auto calculate based on duration")
	rootCmd.PersistentFlags().IntVarP(&duration, "duration", "d", 25, "expected test duration in seconds, used to calculate file size if not specified")
	rootCmd.PersistentFlags().StringVarP(&tempDir, "temp-dir", "t", os.TempDir(), "directory to store temporary files")
	rootCmd.PersistentFlags().StringVarP(&workers, "workers", "w", "100KB,500KB", "worker bandwidth configuration, comma-separated list of bandwidth limits (e.g., '200KB' for one worker, '200KB,200KB,300KB' for three workers)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "", false, "show detailed output from external processes")
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

	fftsPath := GetExecutablePath("ffts")
	serverArgs := []string{
		"--bind-addr", serverAddr,
	}

	serverProcess := NewProcess("Server", fftsPath, serverArgs, verbose)
	err = serverProcess.Start()
	if err != nil {
		return fmt.Errorf("failed to start server process: %v", err)
	}
	defer serverProcess.Stop()

	fmt.Printf("Server started on %s\n", serverAddr)
	time.Sleep(1 * time.Second)

	if !verbose {
		if output := serverProcess.ErrorOutput(); len(output) > 0 {
			fmt.Printf("Server startup warnings/errors: %s\n", output)
		}
	}

	workerProcesses := make([]*Process, 0, len(workerRates))
	workerAddresses := make([]string, 0, len(workerRates))

	fftwPath := GetExecutablePath("fftw")

	for i, rate := range workerRates {
		workerPort, err := allocPort()
		if err != nil {
			return fmt.Errorf("failed to allocate worker%d port: %v", i+1, err)
		}
		workerAddr := fmt.Sprintf("127.0.0.1:%d", workerPort)
		workerAddresses = append(workerAddresses, workerAddr)

		workerArgs := []string{
			"--server-addr", serverAddr,
			"--bind-addr", workerAddr,
			"--advice-public-ip", "127.0.0.1",
			"--rate", fmt.Sprintf("%d", rate),
		}

		workerName := fmt.Sprintf("Worker%d", i+1)
		workerProcess := NewProcess(workerName, fftwPath, workerArgs, verbose)
		err = workerProcess.Start()
		if err != nil {
			return fmt.Errorf("failed to start worker%d process: %v", i+1, err)
		}
		workerProcesses = append(workerProcesses, workerProcess)
		defer workerProcess.Stop()

		fmt.Printf("%s started on %s with bandwidth limit %dKB/s\n", workerName, workerAddr, rate)
	}

	time.Sleep(2 * time.Second)

	if !verbose {
		for i, process := range workerProcesses {
			if output := process.ErrorOutput(); len(output) > 0 {
				fmt.Printf("Worker%d startup warnings/errors: %s\n", i+1, output)
			}
		}
	}

	time.Sleep(2 * time.Second)

	if fileSize == 0 {
		expectedSpeed := float64(totalBandwidth) * 1024 * 0.4      // KB/s to bytes/sec with efficiency factor
		fileSize = int64(expectedSpeed * float64(duration) / 1024) // Convert bytes to KB
		fmt.Printf("Auto-calculated file size: %d KB (%.2f MB) for %d seconds test\n",
			fileSize, float64(fileSize)/1024, duration)
		fmt.Printf("Based on total bandwidth of %dKB/s across %d workers\n",
			totalBandwidth, len(workerRates))
	}

	fileSizeBytes := fileSize * 1024

	testFilePath := filepath.Join(testDir, "test-file")
	err = createTestFile(testFilePath, fileSizeBytes)
	if err != nil {
		return fmt.Errorf("failed to create test file: %v", err)
	}

	recvDir := filepath.Join(testDir, "recv")
	err = os.MkdirAll(recvDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create receive directory: %v", err)
	}

	transferID := fmt.Sprintf("bandwidth-test-%d", time.Now().UnixNano())

	recvBar := pb.New(int(fileSizeBytes))
	recvBar.ShowSpeed = true
	recvBar.SetUnits(pb.U_BYTES)
	recvBar.Start()

	// We don't need the callback function anymore since we're using external processes
	// Progress will be tracked by the external processes

	fmt.Println("Starting file transfer...")
	startTime := time.Now()

	var totalBytes int64

	finfo, err := os.Stat(testFilePath)
	if err != nil {
		return fmt.Errorf("failed to stat test file: %v", err)
	}

	bar := pb.New(int(finfo.Size()))
	bar.ShowSpeed = true
	bar.SetUnits(pb.U_BYTES)
	bar.Start()

	// We don't need the callback function anymore since we're using external processes
	// Progress will be tracked by the external processes

	senderDoneCh := make(chan error, 1)
	receiverDoneCh := make(chan error, 1)

	fmt.Println("Starting sender...")

	fftPath := GetExecutablePath("fft")
	senderArgs := []string{
		"--server-addr", serverAddr,
		"--id", transferID,
		"--send-file", testFilePath,
		"--frame-size", fmt.Sprintf("%d", 5*1024),
		"--cache-count", "512",
	}

	if verbose {
		senderArgs = append(senderArgs, "--debug")
	}

	senderProcess := NewProcess("Sender", fftPath, senderArgs, verbose)

	go func() {
		err := senderProcess.Start()
		if err != nil {
			senderDoneCh <- fmt.Errorf("failed to start sender process: %v", err)
			return
		}

		err = senderProcess.cmd.Wait()
		if err != nil {
			senderDoneCh <- fmt.Errorf("sender process error: %v", err)
			return
		}

		if output := senderProcess.ErrorOutput(); strings.Contains(output, "error") {
			senderDoneCh <- fmt.Errorf("sender error: %s", output)
			return
		}

		bar.Finish()

		senderDoneCh <- nil
	}()

	time.Sleep(2 * time.Second)

	fmt.Println("Starting receiver...")

	receiverArgs := []string{
		"--server-addr", serverAddr,
		"--id", transferID,
		"--recv-file", recvDir,
		"--cache-count", "512",
	}

	if verbose {
		receiverArgs = append(receiverArgs, "--debug")
	}

	receiverProcess := NewProcess("Receiver", fftPath, receiverArgs, verbose)

	go func() {
		err := receiverProcess.Start()
		if err != nil {
			receiverDoneCh <- fmt.Errorf("failed to start receiver process: %v", err)
			return
		}

		err = receiverProcess.cmd.Wait()
		if err != nil {
			receiverDoneCh <- fmt.Errorf("receiver process error: %v", err)
			return
		}

		if output := receiverProcess.ErrorOutput(); strings.Contains(output, "error") {
			receiverDoneCh <- fmt.Errorf("receiver error: %s", output)
			return
		}

		recvBar.Finish()

		receiverDoneCh <- nil
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

	receivedFilePath := filepath.Join(recvDir, filepath.Base(testFilePath))
	receivedInfo, err := os.Stat(receivedFilePath)
	if err != nil {
		return fmt.Errorf("failed to stat received file: %v", err)
	}
	totalBytes = receivedInfo.Size()

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	bytesPerSecond := float64(totalBytes) / duration.Seconds()
	kbPerSecond := bytesPerSecond / 1024

	fmt.Printf("\nBandwidth Test Results:\n")
	fmt.Printf("Total transferred: %d bytes (%.2f KB)\n", totalBytes, float64(totalBytes)/1024)
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
