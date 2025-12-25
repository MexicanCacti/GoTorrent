package handshake

import (
	"GoTorrent/bencode"
	"errors"
	"io"
	"net"
)

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

func deserializeHandshake(buf []byte) *Handshake {
	h := Handshake{}
	PstrLen := int(buf[0])
	curr := 1
	h.Pstr = string(buf[curr : curr+PstrLen])
	curr += PstrLen
	//reserveBytes := buf[curr : curr+8]
	curr += 8
	copy(h.InfoHash[:], buf[curr:curr+20])
	curr += 20
	copy(h.PeerID[:], buf[curr:curr+20])
	return &h
}

func DoHandshake(conn net.Conn, protocolID string, torrent *bencode.TorrentType) (*Handshake, error) {

	handshake := Handshake{
		protocolID,
		torrent.InfoHash,
		torrent.PeerID,
	}

	_, err := conn.Write(handshake.serialize())
	if err != nil {
		return nil, errors.New("handshake safeio failed: " + err.Error())
	}

	buf := make([]byte, len(protocolID)+49)
	if len(buf) < 49 || len(buf) < 49+int(buf[0]) {
		return nil, errors.New("handshake message too short")
	}

	_, err = io.ReadFull(conn, buf)
	if err != nil {
		return nil, errors.New("handshake read failed: " + err.Error())
	}

	handshakeResponse := deserializeHandshake(buf)

	if handshakeResponse.Pstr != protocolID {
		return nil, errors.New("invalid protocol identifier")
	}

	if handshakeResponse.InfoHash != torrent.InfoHash {
		return nil, errors.New("infohash mismatch")
	}

	return handshakeResponse, nil
}
