package msg

const (
	TypeRegisterWorker           = 'a'
	TypeRegisterWorkerResp       = 'b'
	TypeSendFile                 = 'c'
	TypeSendFileResp             = 'd'
	TypeReceiveFile              = 'e'
	TypeReceiveFileResp          = 'f'
	TypeNewSendFileStream        = 'g'
	TypeNewSendFileStreamResp    = 'h'
	TypeNewReceiveFileStream     = 'i'
	TypeNewReceiveFileStreamResp = 'j'

	TypePing = 'y'
	TypePong = 'z'
)

var (
	msgTypeMap = map[byte]interface{}{
		TypeRegisterWorker:           RegisterWorker{},
		TypeRegisterWorkerResp:       RegisterWorkerResp{},
		TypeSendFile:                 SendFile{},
		TypeSendFileResp:             SendFileResp{},
		TypeReceiveFile:              ReceiveFile{},
		TypeReceiveFileResp:          ReceiveFileResp{},
		TypeNewSendFileStream:        NewSendFileStream{},
		TypeNewSendFileStreamResp:    NewSendFileStreamResp{},
		TypeNewReceiveFileStream:     NewReceiveFileStream{},
		TypeNewReceiveFileStreamResp: NewReceiveFileStreamResp{},

		TypePing: Ping{},
		TypePong: Pong{},
	}
)

type RegisterWorker struct {
	Version  string `json:"version"`
	BindPort int64  `json:"bind_port"`
	PublicIP string `json:"public_ip"`
}

type RegisterWorkerResp struct {
	Error string `json:"error"`
}

type SendFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SendFileResp struct {
	ID      string   `json:"id"`
	Workers []string `json:"workers"`
	Error   string   `json:"error"`
}

type ReceiveFile struct {
	ID string `json:"id"`
}

type ReceiveFileResp struct {
	Name    string   `json:"name"`
	Workers []string `json:"workers"`
	Error   string   `json:"error"`
}

type NewSendFileStream struct {
	ID string `json:"id"`
}

type NewSendFileStreamResp struct {
	Error string `json:"error"`
}

type NewReceiveFileStream struct {
	ID string `json:"id"`
}

type NewReceiveFileStreamResp struct {
	Error string `json:"error"`
}

type Ping struct {
}

type Pong struct {
}
