package main

import (
	"fmt"
	"os"

	"github.com/fatedier/fft/client"
	"github.com/fatedier/fft/version"

	"github.com/spf13/cobra"
)

var (
	showVersion bool
	options     client.Options
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of fft client")
	rootCmd.PersistentFlags().StringVarP(&options.ServerAddr, "server_addr", "s", version.DefaultServerAddr(), "remote fft server address")
	rootCmd.PersistentFlags().StringVarP(&options.ID, "id", "i", "", "specify a special id to transfer file")
	rootCmd.PersistentFlags().StringVarP(&options.SendFile, "send_file", "l", "", "specify which file to send to another client")
	rootCmd.PersistentFlags().IntVarP(&options.FrameSize, "frame_size", "n", 5*1024, "each frame size, it's only for sender, default(5*1024 B)")
	rootCmd.PersistentFlags().IntVarP(&options.CacheCount, "cache_count", "c", 512, "how many frames be cached, it will be set to the min value between sender and receiver")
	rootCmd.PersistentFlags().StringVarP(&options.RecvFile, "recv_file", "t", "", "specify local file path to store received file")
	rootCmd.PersistentFlags().BoolVarP(&options.DebugMode, "debug", "g", false, "print more debug info")
}

var rootCmd = &cobra.Command{
	Use:   "fft",
	Short: "fft is the client of fft (https://github.com/fatedier/fft)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(version.Full())
			return nil
		}

		svc, err := client.NewService(options)
		if err != nil {
			fmt.Printf("new fft client error: %v\n", err)
			os.Exit(1)
		}

		err = svc.Run()
		if err != nil {
			fmt.Printf("fft run error: %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}
