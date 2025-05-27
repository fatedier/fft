package basic

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/fft/test/e2e/framework"
)

var _ = ginkgo.Describe("[Feature: Basic]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("File Transfer", func() {
		ginkgo.It("Should transfer a small file successfully", func() {
			testContent := make([]byte, 10*1024)
			for i := range testContent {
				testContent[i] = byte(i % 256)
			}
			
			testFilePath, err := f.CreateTestFile(testContent)
			framework.ExpectNoError(err)
			
			serverPort := f.AllocPort()
			serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
			
			_, serverOutput, err := f.RunServer("--bind_addr", serverAddr)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Server started on %s\n", serverAddr)
			ginkgo.GinkgoWriter.Printf("Server output: %s\n", serverOutput)
			
			workerPort := f.AllocPort()
			workerAddr := fmt.Sprintf("127.0.0.1:%d", workerPort)
			_, workerOutput, err := f.RunWorker("--server_addr", serverAddr, "--bind_addr", workerAddr)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Worker connected to server\n")
			ginkgo.GinkgoWriter.Printf("Worker output: %s\n", workerOutput)
			
			transferID := fmt.Sprintf("test-%d", time.Now().UnixNano())
			
			recvDir := filepath.Join(f.TempDirectory, "recv")
			err = os.MkdirAll(recvDir, 0755)
			framework.ExpectNoError(err)
			
			ginkgo.GinkgoWriter.Printf("Starting sender with ID: %s\n", transferID)
			go func() {
				_, senderOutput, err := f.RunClient(
					"-s", serverAddr,
					"-i", transferID,
					"-l", testFilePath,
					"-g", "true", // debug mode
				)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("Sender error: %v\n", err)
				}
				ginkgo.GinkgoWriter.Printf("Sender output: %s\n", senderOutput)
			}()
			
			time.Sleep(2 * time.Second)
			
			ginkgo.GinkgoWriter.Printf("Starting receiver with ID: %s\n", transferID)
			_, receiverOutput, err := f.RunClient(
				"-s", serverAddr,
				"-i", transferID,
				"-t", recvDir,
				"-g", "true", // debug mode
			)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Receiver output: %s\n", receiverOutput)
			
			time.Sleep(5 * time.Second)
			
			receivedFilePath := filepath.Join(recvDir, filepath.Base(testFilePath))
			err = f.VerifyFileContent(receivedFilePath, testContent)
			framework.ExpectNoError(err, "File content verification failed")
		})

		ginkgo.It("Should transfer a medium file successfully", func() {
			testContent := make([]byte, 1024*1024)
			for i := range testContent {
				testContent[i] = byte(i % 256)
			}
			
			testFilePath, err := f.CreateTestFile(testContent)
			framework.ExpectNoError(err)
			
			serverPort := f.AllocPort()
			serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
			
			_, serverOutput, err := f.RunServer("--bind_addr", serverAddr)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Server started on %s\n", serverAddr)
			ginkgo.GinkgoWriter.Printf("Server output: %s\n", serverOutput)
			
			workerPort := f.AllocPort()
			workerAddr := fmt.Sprintf("127.0.0.1:%d", workerPort)
			_, workerOutput, err := f.RunWorker("--server_addr", serverAddr, "--bind_addr", workerAddr)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Worker connected to server\n")
			ginkgo.GinkgoWriter.Printf("Worker output: %s\n", workerOutput)
			
			transferID := fmt.Sprintf("test-%d", time.Now().UnixNano())
			
			recvDir := filepath.Join(f.TempDirectory, "recv-medium")
			err = os.MkdirAll(recvDir, 0755)
			framework.ExpectNoError(err)
			
			ginkgo.GinkgoWriter.Printf("Starting sender with ID: %s\n", transferID)
			go func() {
				_, senderOutput, err := f.RunClient(
					"-s", serverAddr,
					"-i", transferID,
					"-l", testFilePath,
					"-g", "true", // debug mode
				)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("Sender error: %v\n", err)
				}
				ginkgo.GinkgoWriter.Printf("Sender output: %s\n", senderOutput)
			}()
			
			time.Sleep(2 * time.Second)
			
			ginkgo.GinkgoWriter.Printf("Starting receiver with ID: %s\n", transferID)
			_, receiverOutput, err := f.RunClient(
				"-s", serverAddr,
				"-i", transferID,
				"-t", recvDir,
				"-g", "true", // debug mode
			)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Receiver output: %s\n", receiverOutput)
			
			time.Sleep(10 * time.Second)
			
			receivedFilePath := filepath.Join(recvDir, filepath.Base(testFilePath))
			err = f.VerifyFileContent(receivedFilePath, testContent)
			framework.ExpectNoError(err, "File content verification failed")
		})

		ginkgo.It("Should transfer a file with custom frame size and cache count", func() {
			testContent := make([]byte, 100*1024)
			for i := range testContent {
				testContent[i] = byte(i % 256)
			}
			
			testFilePath, err := f.CreateTestFile(testContent)
			framework.ExpectNoError(err)
			
			serverPort := f.AllocPort()
			serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
			
			_, serverOutput, err := f.RunServer("--bind_addr", serverAddr)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Server started on %s\n", serverAddr)
			ginkgo.GinkgoWriter.Printf("Server output: %s\n", serverOutput)
			
			workerPort := f.AllocPort()
			workerAddr := fmt.Sprintf("127.0.0.1:%d", workerPort)
			_, workerOutput, err := f.RunWorker("--server_addr", serverAddr, "--bind_addr", workerAddr)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Worker connected to server\n")
			ginkgo.GinkgoWriter.Printf("Worker output: %s\n", workerOutput)
			
			transferID := fmt.Sprintf("test-%d", time.Now().UnixNano())
			
			recvDir := filepath.Join(f.TempDirectory, "recv-custom")
			err = os.MkdirAll(recvDir, 0755)
			framework.ExpectNoError(err)
			
			ginkgo.GinkgoWriter.Printf("Starting sender with ID: %s\n", transferID)
			go func() {
				_, senderOutput, err := f.RunClient(
					"-s", serverAddr,
					"-i", transferID,
					"-l", testFilePath,
					"-n", "10240", // 10KB frame size
					"-c", "256",   // 256 frames cache
					"-g", "true",  // debug mode
				)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("Sender error: %v\n", err)
				}
				ginkgo.GinkgoWriter.Printf("Sender output: %s\n", senderOutput)
			}()
			
			time.Sleep(2 * time.Second)
			
			ginkgo.GinkgoWriter.Printf("Starting receiver with ID: %s\n", transferID)
			_, receiverOutput, err := f.RunClient(
				"-s", serverAddr,
				"-i", transferID,
				"-t", recvDir,
				"-c", "256",  // 256 frames cache
				"-g", "true", // debug mode
			)
			framework.ExpectNoError(err)
			ginkgo.GinkgoWriter.Printf("Receiver output: %s\n", receiverOutput)
			
			time.Sleep(5 * time.Second)
			
			receivedFilePath := filepath.Join(recvDir, filepath.Base(testFilePath))
			err = f.VerifyFileContent(receivedFilePath, testContent)
			framework.ExpectNoError(err, "File content verification failed")
		})
	})
})
