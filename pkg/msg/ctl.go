package msg

import (
	"io"

	jsonMsg "github.com/fatedier/golib/msg/json"
)

type Message = jsonMsg.Message

var (
	msgCtl *jsonMsg.MsgCtl
)

func init() {
	msgCtl = jsonMsg.NewMsgCtl()
	for typeByte, msg := range msgTypeMap {
		msgCtl.RegisterMsg(typeByte, msg)
	}
}

func ReadMsg(c io.Reader) (msg Message, err error) {
	return msgCtl.ReadMsg(c)
}

func ReadMsgInto(c io.Reader, msg Message) (err error) {
	return msgCtl.ReadMsgInto(c, msg)
}

func WriteMsg(c io.Writer, msg interface{}) (err error) {
	return msgCtl.WriteMsg(c, msg)
}
