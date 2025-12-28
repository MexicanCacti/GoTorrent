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

/*
	transactionID := rand.Uint32()

	// Connect input
	buf := new(bytes.Buffer)
	action := uint32(0)

	safeWriter := safeio.NewSafeWriter(buf)
	safeWriter.WriteBigEndian(protocolID)
	safeWriter.WriteBigEndian(action)
	safeWriter.WriteBigEndian(transactionID)

	if safeWriter.GetError() != nil {
		return nil, safeWriter.GetError()
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	resp := make([]byte, 16)
	err = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err != nil {
		log.Fatal(err)
	}

	// Connect output
	n, err := conn.Read(resp)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	conn.SetReadDeadline(time.Time{})
	if n < 16 {
		return nil, fmt.Errorf("invalid response length: %d", n)
	}

	respBuf := bytes.NewBuffer(resp)
	var respAction uint32
	var respTransactionID uint32
	var respConnectionID uint64

	safeReader := safeio.NewSafeReader(respBuf)
	safeReader.ReadBigEndian(&respAction)
	safeReader.ReadBigEndian(&respTransactionID)
	safeReader.ReadBigEndian(&respConnectionID)

	if safeReader.GetError() != nil {
		return nil, safeReader.GetError()
	}

	if respAction != 0 {
		return nil, fmt.Errorf("invalid response action %d", respAction)
	}
	if respTransactionID != transactionID {
		return nil, fmt.Errorf("invalid transaction ID %d", respTransactionID)
	}

	transactionID = rand.Uint32()
	// Announce input
	buf = new(bytes.Buffer)

	safeWriter = safeio.NewSafeWriter(buf)
	safeWriter.WriteBigEndian(respConnectionID)
	safeWriter.WriteBigEndian(uint32(1)) // action
	safeWriter.WriteBigEndian(transactionID)

	buf.Write(t.InfoHash[:])
	buf.Write(peerID[:])

	safeWriter.WriteBigEndian(uint64(0))        // downloaded
	safeWriter.WriteBigEndian(uint64(t.Length)) // left
	safeWriter.WriteBigEndian(uint64(0))        // uploaded
	safeWriter.WriteBigEndian(uint32(0))        // event
	safeWriter.WriteBigEndian(uint32(0))        // IP address
	safeWriter.WriteBigEndian(rand.Uint32())    // key
	safeWriter.WriteBigEndian(int32(-1))        // num_want
	safeWriter.WriteBigEndian(port)

	if safeWriter.GetError() != nil {
		return nil, safeWriter.GetError()
	}

	_, err = conn.Write(buf.Bytes())

	// Announce output
	announceBuf := make([]byte, 1500) // Hopefully good enough, 1472 theoretical max message size
	n, err = conn.Read(announceBuf)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	announceResp := announceBuf[:n]

	fmt.Printf("Size of resp: %v", n)
	var announceAction = binary.BigEndian.Uint32(announceResp[0:4])
	var announceTransactionID = binary.BigEndian.Uint32(announceResp[4:8])

	if announceAction != 1 {
		return nil, fmt.Errorf("invalid announce action %d", announceAction)
	}

	if announceTransactionID != transactionID {
		return nil, fmt.Errorf("invalid transaction ID %d", announceTransactionID)
	}

	var announceInterval = binary.BigEndian.Uint32(announceResp[8:12])
	var announceLeechers = binary.BigEndian.Uint32(announceResp[12:16])
	var announceSeeders = binary.BigEndian.Uint32(announceResp[16:20])

	fmt.Printf("\nLeechers : %v |\t", announceLeechers)
	fmt.Printf("Seeders: %v\n", announceSeeders)
	trackerResponse := udpResponse{}
	trackerResponse.Interval = uint64(announceInterval)
	trackerResponse.Peers = announceResp[20:]

	return udpExtractPeers(&trackerResponse)
*/
