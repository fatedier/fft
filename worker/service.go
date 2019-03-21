package worker

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"time"

	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"
)

type Options struct {
	ServerAddr         string
	BindAddr           string
	AdvicePublicIP     string
	RateKB             int // xx KB/s
	MaxTrafficMBPerDay int // xx MB, 0 is no limit

	LogFile    string
	LogLevel   string
	LogMaxDays int64
}

func (op *Options) Check() error {
	if op.LogMaxDays <= 0 {
		op.LogMaxDays = 3
	}
	if op.RateKB < 50 {
		return fmt.Errorf("rate should be greater than 50KB")
	}
	if op.MaxTrafficMBPerDay < 128 && op.MaxTrafficMBPerDay != 0 {
		return fmt.Errorf("max_traffic_per_day should be greater than 128MB")
	}
	return nil
}

type Service struct {
	serverAddr         string
	advicePublicIP     string
	rateKB             int
	maxTrafficMBPerDay int

	l              net.Listener
	matchCtl       *MatchController
	register       *Register
	trafficLimiter *TrafficLimiter
	tlsConfig      *tls.Config

	stopCh chan struct{}
}

func NewService(options Options) (*Service, error) {
	if err := options.Check(); err != nil {
		return nil, err
	}

	logway := "file"
	if options.LogFile == "console" {
		logway = "console"
	}
	log.InitLog(logway, options.LogFile, options.LogLevel, options.LogMaxDays)

	l, err := net.Listen("tcp", options.BindAddr)
	if err != nil {
		return nil, err
	}
	log.Info("fftw listen on: %s", l.Addr().String())

	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return nil, fmt.Errorf("get bind port error, bind address: %v", l.Addr().String())
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("get bind port error: %v", err)
	}

	register, err := NewRegister(int64(port), options.AdvicePublicIP, options.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("new register error: %v", err)
	}

	svc := &Service{
		serverAddr:         options.ServerAddr,
		advicePublicIP:     options.AdvicePublicIP,
		rateKB:             options.RateKB,
		maxTrafficMBPerDay: options.MaxTrafficMBPerDay,

		l:         l,
		register:  register,
		tlsConfig: generateTLSConfig(),

		stopCh: make(chan struct{}),
	}

	svc.trafficLimiter = NewTrafficLimiter(uint64(options.MaxTrafficMBPerDay*1024*1024), func() {
		svc.register.Close()
		log.Info("reach traffic limit %dMB one day, unregister from server", options.MaxTrafficMBPerDay)
	}, func() {
		svc.register.Reset()
		go svc.register.RunKeepAlive()
		log.Info("restore from traffic limit since it's a new day")
	})

	svc.matchCtl = NewMatchController(options.RateKB*1024, func(n int) {
		svc.trafficLimiter.AddCount(uint64(n))
	})
	return svc, nil
}

func (svc *Service) Run() error {
	go svc.worker()
	go svc.trafficLimiter.Run()

	err := svc.register.Register()
	if err != nil {
		return fmt.Errorf("register worker to server error: %v", err)
	}
	log.Info("register to server success")

	svc.register.RunKeepAlive()
	<-svc.stopCh
	return nil
}

func (svc *Service) worker() error {
	for {
		conn, err := svc.l.Accept()
		if err != nil {
			return err
		}
		conn = tls.Server(conn, svc.tlsConfig)
		go svc.handleConn(conn)
	}
}

func (svc *Service) handleConn(conn net.Conn) {
	var (
		rawMsg msg.Message
		err    error
	)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if rawMsg, err = msg.ReadMsg(conn); err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	switch m := rawMsg.(type) {
	case *msg.NewSendFileStream:
		log.Debug("new send file stream [%s]", m.ID)
		tc := NewTransferConn(m.ID, conn, true)
		if err = svc.matchCtl.DealTransferConn(tc, 20*time.Second); err != nil {
			msg.WriteMsg(conn, &msg.NewSendFileStreamResp{
				Error: err.Error(),
			})
			conn.Close()
		}
	case *msg.NewReceiveFileStream:
		log.Debug("new recv file stream [%s]", m.ID)
		tc := NewTransferConn(m.ID, conn, false)
		if err = svc.matchCtl.DealTransferConn(tc, 20*time.Second); err != nil {
			msg.WriteMsg(conn, &msg.NewReceiveFileStreamResp{
				Error: err.Error(),
			})
			conn.Close()
		}
	case *msg.Ping:
		log.Debug("return pong to server ping")
		msg.WriteMsg(conn, &msg.Pong{})
		conn.Close()
		return
	default:
		conn.Close()
		return
	}
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}
}
