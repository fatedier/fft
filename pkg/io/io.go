package io

import (
	"io"
)

type CallbackReader struct {
	r        io.Reader
	callback func(n int)
}

func NewCallbackReader(r io.Reader, callback func(n int)) *CallbackReader {
	return &CallbackReader{
		r:        r,
		callback: callback,
	}
}

func (cr *CallbackReader) Read(p []byte) (n int, err error) {
	n, err = cr.r.Read(p)
	cr.callback(n)
	return
}

type CallbackWriter struct {
	w        io.Writer
	callback func(n int)
}

func NewCallbackWriter(w io.Writer, callback func(n int)) *CallbackWriter {
	return &CallbackWriter{
		w:        w,
		callback: callback,
	}
}

func (cw *CallbackWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.callback(n)
	return
}
