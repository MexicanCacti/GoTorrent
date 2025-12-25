package safeio

import (
	"encoding/binary"
	"io"
)

type SafeReader struct {
	r   io.Reader
	err error
}

func NewSafeReader(r io.Reader) *SafeReader {
	return &SafeReader{r: r}
}

func (reader *SafeReader) GetError() error {
	return reader.err
}

func (reader *SafeReader) ReadBigEndian(data any) {
	if reader.err != nil {
		return
	}

	reader.err = binary.Read(reader.r, binary.BigEndian, data)
}
