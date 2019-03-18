package stream

func IsValidFrameSize(frameSize int) bool {
	if frameSize <= 0 || frameSize > 65535 {
		return false
	}
	return true
}

type Frame struct {
	Version uint8
	FileID  uint32
	FrameID uint32
	Buf     []byte // if len(Buf) == 0 , is last frame
}

func NewFrame(fileID uint32, frameID uint32, buf []byte) *Frame {
	return &Frame{
		Version: 0,
		FileID:  fileID,
		FrameID: frameID,
		Buf:     buf,
	}
}

type Ack struct {
	Version uint8
	FileID  uint32
	FrameID uint32
}

func NewAck(fileID uint32, frameID uint32) *Ack {
	return &Ack{
		Version: 0,
		FileID:  fileID,
		FrameID: frameID,
	}
}
