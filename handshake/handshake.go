package handshake

import (
	"GoTorrent/bencode"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const handshakeWaitFactor = 5

type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
}

func (h *Handshake) serialize() []byte {
	buf := make([]byte, len(h.Pstr)+49)
	buf[0] = byte(len(h.Pstr))
	curr := 1
	curr += copy(buf[curr:], h.Pstr)
	curr += copy(buf[curr:], make([]byte, 8)) // Reserved, flip to indicate ext supported
	curr += copy(buf[curr:], h.InfoHash[:])
	curr += copy(buf[curr:], h.PeerID[:])
	return buf
}

func deserializeHandshake(conn net.Conn) (*Handshake, error) {
	lengthBuf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(handshakeWaitFactor * time.Second))
	_, err := io.ReadFull(conn, lengthBuf)
	defer conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, err
	}
	pStrLen := int(lengthBuf[0])

	if pStrLen == 0 {
		return nil, fmt.Errorf("ptrstrlen was 0")
	}

	handshakeBuf := make([]byte, pStrLen+48)
	conn.SetReadDeadline(time.Now().Add(handshakeWaitFactor * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	_, err = io.ReadFull(conn, handshakeBuf)
	if err != nil {
		return nil, err
	}

	h := Handshake{}
	h.Pstr = string(handshakeBuf[0:pStrLen])
	copy(h.InfoHash[:], handshakeBuf[pStrLen+8:pStrLen+20+8])
	copy(h.PeerID[:], handshakeBuf[pStrLen+8+20:])
	return &h, nil
}

func DoHandshake(conn net.Conn, protocolID string, torrent *bencode.TorrentType) (*Handshake, error) {

	handshake := Handshake{
		protocolID,
		torrent.InfoHash,
		torrent.PeerID,
	}

	conn.SetWriteDeadline(time.Now().Add(handshakeWaitFactor * time.Second))
	defer conn.SetWriteDeadline(time.Time{})
	_, err := conn.Write(handshake.serialize())
	if err != nil {
		return nil, errors.New("handshake write failed: " + err.Error())
	}

	handshakeResponse, err := deserializeHandshake(conn)
	if err != nil {
		return nil, errors.New("handshake deserialize failed: " + err.Error())
	}

	if handshakeResponse.Pstr != protocolID {
		return nil, errors.New("invalid protocol identifier")
	}

	if handshakeResponse.InfoHash != torrent.InfoHash {
		return nil, errors.New("infohash mismatch")
	}

	return handshakeResponse, nil
}
