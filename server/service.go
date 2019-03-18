package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"
)

type Options struct {
	BindAddr string

	LogFile    string
	LogLevel   string
	LogMaxDays int64
}

func (op *Options) Check() error {
	if op.LogMaxDays <= 0 {
		op.LogMaxDays = 3
	}
	return nil
}

type Service struct {
	l               net.Listener
	workerGroup     *WorkerGroup
	matchController *MatchController

	tlsConfig *tls.Config
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
	log.Info("ffts listen on: %s", l.Addr().String())

	return &Service{
		l:               l,
		workerGroup:     NewWorkerGroup(),
		matchController: NewMatchController(),
		tlsConfig:       generateTLSConfig(),
	}, nil
}

func (svc *Service) Run() error {
	// Debug ========
	go func() {
		for {
			time.Sleep(10 * time.Second)
			log.Info("worker addrs: %v", svc.workerGroup.GetAvailableWorkerAddrs())
		}
	}()
	// Debug ========

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
	case *msg.RegisterWorker:
		err = svc.handleRegisterWorker(conn, m)
		if err != nil {
			msg.WriteMsg(conn, &msg.RegisterWorkerResp{
				Error: err.Error(),
			})
			conn.Close()
		}
	case *msg.SendFile:
		if err = svc.handleSendFile(conn, m); err != nil {
			msg.WriteMsg(conn, &msg.SendFileResp{
				Error: err.Error(),
			})
			conn.Close()
		}
	case *msg.ReceiveFile:
		if err = svc.handleRecvFile(conn, m); err != nil {
			msg.WriteMsg(conn, &msg.ReceiveFileResp{
				Error: err.Error(),
			})
			conn.Close()
		}
	default:
		conn.Close()
		return
	}
}

func (svc *Service) handleRegisterWorker(conn net.Conn, m *msg.RegisterWorker) error {
	log.Debug("get register worker: remote addr [%s] port [%d], advice public IP [%s]",
		conn.RemoteAddr().String(), m.BindPort, m.PublicIP)
	w := NewWorker(m.BindPort, m.PublicIP, conn)
	err := w.DetectPublicAddr()
	if err != nil {
		log.Warn("detect [%s] public address error: %v", conn.RemoteAddr().String(), err)
		return err
	} else {
		msg.WriteMsg(conn, &msg.RegisterWorkerResp{Error: ""})
	}

	svc.workerGroup.RegisterWorker(w)
	log.Info("[%s] new worker register", w.PublicAddr())
	return nil
}

func (svc *Service) handleSendFile(conn net.Conn, m *msg.SendFile) error {
	if m.ID == "" || m.Name == "" {
		return fmt.Errorf("id and file name is required")
	}
	log.Debug("new SendFile id [%s], filename [%s] size [%d]", m.ID, m.Name, m.Fsize)

	sc := NewSendConn(m.ID, conn, m.Name, m.Fsize, m.CacheCount)
	cacheCount, err := svc.matchController.DealSendConn(sc, 120*time.Second)
	if err != nil {
		log.Warn("deal send conn error: %v", err)
		return err
	}

	msg.WriteMsg(conn, &msg.SendFileResp{
		ID:         m.ID,
		Workers:    svc.workerGroup.GetAvailableWorkerAddrs(),
		CacheCount: cacheCount,
	})
	return nil
}

func (svc *Service) handleRecvFile(conn net.Conn, m *msg.ReceiveFile) error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	log.Debug("new ReceiveFile id [%s]", m.ID)

	rc := NewRecvConn(m.ID, conn, m.CacheCount)
	filename, fsize, cacheCount, err := svc.matchController.DealRecvConn(rc)
	if err != nil {
		log.Warn("deal recv conn error: %v", err)
		return err
	}

	msg.WriteMsg(conn, &msg.ReceiveFileResp{
		Name:       filename,
		Fsize:      fsize,
		Workers:    svc.workerGroup.GetAvailableWorkerAddrs(),
		CacheCount: cacheCount,
	})
	return nil
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
