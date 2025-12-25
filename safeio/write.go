package safeio

import (
	"encoding/binary"
	"io"
)

type SafeWriter struct {
	w   io.Writer
	err error
}

func NewSafeWriter(w io.Writer) *SafeWriter {
	return &SafeWriter{w: w}
}

func (writer *SafeWriter) GetError() error {
	return writer.err
}

func (writer *SafeWriter) Write(p []byte) (int, error) {
	if writer.err != nil {
		return 0, writer.err
	}

	n, err := writer.w.Write(p)
	if err != nil {
		writer.err = err
	}
	return n, err
}

func (writer *SafeWriter) WriteBigEndian(data any) {
	if writer.err != nil {
		return
	}
	writer.err = binary.Write(writer.w, binary.BigEndian, data)
}
