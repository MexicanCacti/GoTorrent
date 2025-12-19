package networking

import (
	"GoTorrent/handshake"
	"GoTorrent/message"
	"GoTorrent/torrentstruct"
	"errors"
	"net"
	"time"
)

const connectionWaitFactor = 3
const protocolIdentifier = "BitTorrent protocol"

type Bitfield []byte // 0 indexed... 0b110, piece 2 is missing, 0b011, piece 0 is missing, big endian
// Size: math.ceil(numPieces / 8)
// Spare: 8 * Size - numPieces
// Client MAY send bitfield, MUST be first msg after handshake

func (bitfield Bitfield) hasPiece(index int) bool {
	byteIndex := index / 8
	byteOffset := index % 8
	return bitfield[byteIndex]>>(7-byteOffset)&1 != 0
}

func (bitfield Bitfield) setPiece(index int) {
	byteIndex := index / 8
	byteOffset := index % 8
	bitfield[byteIndex] |= 1 << (7 - byteOffset)
}

func (bitfield Bitfield) Pieces() []int {
	var pieces []int

	for i := 0; i < len(bitfield)*8; i++ {
		if bitfield.hasPiece(i) {
			pieces = append(pieces, i)
		}
	}

	return pieces
}

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield Bitfield
	peer     Peer
	infoHash [20]byte
	peerID   [20]byte
}

func New(peer Peer, torrent *torrentstruct.TorrentType) (*Client, error) {
	dialer := net.Dialer{
		Timeout: connectionWaitFactor * time.Second,
	}
	peerAddress := peer.GetTCPAddress()
	conn, err := dialer.Dial("tcp", peerAddress)
	if err != nil {
		return nil, err
	}

	handshakeResponse, err := handshake.DoHandshake(conn, protocolIdentifier, torrent)
	if err != nil {
		return nil, errors.New("handshake failed: " + err.Error())
	}

	bitfieldMsg, err := message.ReadMessage(conn)
	if err != nil {
		return nil, errors.New("bitfield message failed: " + err.Error())
	}

	client := Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: Bitfield(bitfieldMsg.Payload),
		peer:     peer,
		infoHash: torrent.InfoHash,
		peerID:   handshakeResponse.PeerID,
	}

	return &client, nil
}

type PieceProgress struct {
	Index      int
	Client     *Client
	Buf        []byte
	downloaded int
	requested  int
	backlog    int
}
