package main

import (
	"fmt"
	"os"

	"github.com/fatedier/fft/version"
	"github.com/fatedier/fft/worker"

	"github.com/spf13/cobra"
)

var (
	showVersion bool
	options     worker.Options
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of fft worker")
	rootCmd.PersistentFlags().StringVarP(&options.ServerAddr, "server_addr", "s", version.DefaultServerAddr(), "remote fft server address")
	rootCmd.PersistentFlags().StringVarP(&options.BindAddr, "bind_addr", "b", "0.0.0.0:7778", "bind address")
	rootCmd.PersistentFlags().StringVarP(&options.AdvicePublicIP, "advice_public_ip", "p", "", "fft worker's advice public ip")
	rootCmd.PersistentFlags().IntVarP(&options.RateKB, "rate", "", 4096, "max bandwidth fftw will provide, unit is KB, default is 4096KB and min value is 50KB")
	rootCmd.PersistentFlags().IntVarP(&options.MaxTrafficMBPerDay, "max_traffic_per_day", "", 0, "max traffic fftw can use every day, 0 means no limit, unit is MB, default is 0MB and min value is 128MB")

	rootCmd.PersistentFlags().StringVarP(&options.LogFile, "log_file", "", "console", "log file path")
	rootCmd.PersistentFlags().StringVarP(&options.LogLevel, "log_level", "", "info", "log level")
	rootCmd.PersistentFlags().Int64VarP(&options.LogMaxDays, "log_max_days", "", 3, "log file reserved max days")
}

var rootCmd = &cobra.Command{
	Use:   "fftw",
	Short: "fftw is the worker of fft (https://github.com/fatedier/fft)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(version.Full())
			return nil
		}

		svc, err := worker.NewService(options)
		if err != nil {
			fmt.Printf("new fft worker error: %v\n", err)
			os.Exit(1)
		}

		err = svc.Run()
		if err != nil {
			fmt.Printf("fft worker runner exit: %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}
