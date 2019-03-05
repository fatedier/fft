package stream

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

var (
	FrameSize = 51200
)

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

type FrameStream struct {
	conn net.Conn
}

func NewFrameStream(conn net.Conn) *FrameStream {
	return &FrameStream{
		conn: conn,
	}
}

func (fs *FrameStream) WriteFrame(frame *Frame) error {
	buffer := bytes.NewBuffer(nil)
	binary.Write(buffer, binary.BigEndian, uint8(frame.Version))
	binary.Write(buffer, binary.BigEndian, uint32(frame.FileID))
	binary.Write(buffer, binary.BigEndian, uint32(frame.FrameID))
	binary.Write(buffer, binary.BigEndian, uint16(len(frame.Buf)))
	_, err := fs.conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}

	if len(frame.Buf) > 0 {
		_, err = io.Copy(fs.conn, bytes.NewBuffer(frame.Buf))
		if err != nil {
			return err
		}
	}
	return nil
}

func (fs *FrameStream) ReadFrame() (*Frame, error) {
	f := &Frame{}
	err := binary.Read(fs.conn, binary.BigEndian, &f.Version)
	if err != nil {
		return nil, err
	}

	err = binary.Read(fs.conn, binary.BigEndian, &f.FileID)
	if err != nil {
		return nil, err
	}

	err = binary.Read(fs.conn, binary.BigEndian, &f.FrameID)
	if err != nil {
		return nil, err
	}

	var length uint16
	err = binary.Read(fs.conn, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	// last frame
	if length == 0 {
		return f, nil
	}

	f.Buf = make([]byte, length)
	n, err := io.ReadFull(fs.conn, f.Buf)
	if err != nil {
		return nil, err
	}
	f.Buf = f.Buf[:n]

	if uint16(n) != length {
		return nil, fmt.Errorf("error frame length")
	}
	return f, nil
}

func (fs *FrameStream) Close() error {
	return fs.conn.Close()
}
