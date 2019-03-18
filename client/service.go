package client

import (
	"fmt"
)

type Options struct {
	ServerAddr string
	ID         string
	SendFile   string
	FrameSize  int
	CacheCount int
	RecvFile   string
	DebugMode  bool
}

func (op *Options) Check() error {
	if op.SendFile == "" && op.RecvFile == "" {
		return fmt.Errorf("send_file or recv_file is required")
	}

	if op.SendFile != "" {
		if op.FrameSize <= 0 {
			return fmt.Errorf("frame_size should be greater than 0")
		}
	}

	if op.CacheCount <= 0 {
		return fmt.Errorf("cache_count should be greater than 0")
	}
	return nil
}

type Service struct {
	debugMode  bool
	serverAddr string
	frameSize  int
	cacheCount int

	runHandler func() error
}

func NewService(options Options) (*Service, error) {
	if err := options.Check(); err != nil {
		return nil, err
	}

	svc := &Service{
		debugMode:  options.DebugMode,
		serverAddr: options.ServerAddr,
		frameSize:  options.FrameSize,
		cacheCount: options.CacheCount,
	}

	if options.SendFile != "" {
		svc.runHandler = func() error {
			return svc.sendFile(options.ID, options.SendFile)
		}
	} else {
		svc.runHandler = func() error {
			return svc.recvFile(options.ID, options.RecvFile)
		}
	}
	return svc, nil
}

func (svc *Service) Run() error {
	err := svc.runHandler()
	if err != nil && svc.debugMode {
		fmt.Println(err)
	}
	return err
}

func log(debugMode bool, foramt string, v ...interface{}) {
	if debugMode {
		fmt.Printf(foramt+"\n", v...)
	}
}
