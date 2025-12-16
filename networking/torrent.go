package networking

import (
	"GoTorrent/torrentstruct"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const ConnectionWaitFactor = 3
const ProtocolLength = 19
const ProtocolIdentifier = "BitTorrent protocol"

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

func deserialize(buf []byte) *Handshake {
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

func ConnectToPeer(peer Peer, torrent *torrentstruct.TorrentType, wg *sync.WaitGroup) {
	defer wg.Done()
	dialer := net.Dialer{
		Timeout: ConnectionWaitFactor * time.Second,
	}
	peerAddress := peer.GetTCPAddress()
	conn, err := dialer.Dial("tcp", peerAddress)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	handShake := Handshake{
		ProtocolIdentifier,
		torrent.InfoHash,
		torrent.PeerID,
	}

	_, err = conn.Write(handShake.serialize())
	if err != nil {
		log.Println("handshake write failed: ", err)
		return
	}
	buf := make([]byte, len(ProtocolIdentifier)+49)

	_, err = io.ReadFull(conn, buf)
	if err != nil {
		log.Println("handshake read failed: ", err)
		return
	}
	handshakeResponse := deserialize(buf)

	if handshakeResponse.Pstr != ProtocolIdentifier {
		log.Println("invalid protocol identifier")
		return
	}

	if handshakeResponse.InfoHash != torrent.InfoHash {
		log.Printf("InfoHash not match")
		return
	}

	connectionID := handshakeResponse.PeerID

	fmt.Printf("IP: %v | Port: %v | ID: %v\n", peer.IP, peer.Port, connectionID)
}
