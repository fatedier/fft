package main

import (
	"fmt"
	"os"

	"github.com/fatedier/fft/server"
	"github.com/fatedier/fft/version"

	"github.com/spf13/cobra"
)

var (
	showVersion bool
	options     server.Options
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of fft server")
	rootCmd.PersistentFlags().StringVarP(&options.BindAddr, "bind_addr", "b", "0.0.0.0:7777", "bind address")
	rootCmd.PersistentFlags().StringVarP(&options.LogFile, "log_file", "", "console", "log file path")
	rootCmd.PersistentFlags().StringVarP(&options.LogLevel, "log_level", "", "info", "log level")
	rootCmd.PersistentFlags().Int64VarP(&options.LogMaxDays, "log_max_days", "", 3, "log file reserved max days")
}

var rootCmd = &cobra.Command{
	Use:   "ffts",
	Short: "ffts is the server of fft (https://github.com/fatedier/fft)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(version.Full())
			return nil
		}

		svc, err := server.NewService(options)
		if err != nil {
			fmt.Printf("new fft server error: %v", err)
			os.Exit(1)
		}

		svc.Run()
		return nil
	},
}
