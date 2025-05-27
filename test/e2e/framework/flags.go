package framework

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func RegisterCommonFlags(flags *flag.FlagSet) {
	flags.StringVar(&TestContext.FFTPath, "fft-path", "", "Path to fft binary")
	flags.StringVar(&TestContext.FFTSPath, "ffts-path", "", "Path to ffts binary")
	flags.StringVar(&TestContext.FFTWPath, "fftw-path", "", "Path to fftw binary")
	flags.StringVar(&TestContext.LogLevel, "log-level", "info", "Log level")
	flags.BoolVar(&TestContext.Debug, "debug", false, "Debug mode")
}

func ValidateTestContext(testContext *struct {
	FFTPath  string
	FFTSPath string
	FFTWPath string
	LogLevel string
	Debug    bool
}) error {
	if testContext.FFTPath == "" {
		testContext.FFTPath = filepath.Join(os.Getenv("HOME"), "repos", "fft", "bin", "fft")
	}
	if testContext.FFTSPath == "" {
		testContext.FFTSPath = filepath.Join(os.Getenv("HOME"), "repos", "fft", "bin", "ffts")
	}
	if testContext.FFTWPath == "" {
		testContext.FFTWPath = filepath.Join(os.Getenv("HOME"), "repos", "fft", "bin", "fftw")
	}

	if _, err := os.Stat(testContext.FFTPath); os.IsNotExist(err) {
		return fmt.Errorf("fft binary not found at %s", testContext.FFTPath)
	}
	if _, err := os.Stat(testContext.FFTSPath); os.IsNotExist(err) {
		return fmt.Errorf("ffts binary not found at %s", testContext.FFTSPath)
	}
	if _, err := os.Stat(testContext.FFTWPath); os.IsNotExist(err) {
		return fmt.Errorf("fftw binary not found at %s", testContext.FFTWPath)
	}

	return nil
}
