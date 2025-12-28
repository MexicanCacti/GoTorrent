package client

import (
	"GoTorrent/bencode"
	"GoTorrent/handshake"
	"GoTorrent/message"
	"GoTorrent/peer_discovery"
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

func (bitfield Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8
	byteOffset := index % 8
	return bitfield[byteIndex]>>(7-byteOffset)&1 != 0
}

func (bitfield Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	byteOffset := index % 8
	bitfield[byteIndex] |= 1 << (7 - byteOffset)
}

func (bitfield Bitfield) ClearPiece(index int) {
	byteIndex := index / 8
	byteOffset := index % 8
	bitfield[byteIndex] &= ^(1 << (7 - byteOffset))
}

func (bitfield Bitfield) Pieces() []int {
	var pieces []int

	for i := 0; i < len(bitfield)*8; i++ {
		if bitfield.HasPiece(i) {
			pieces = append(pieces, i)
		}
	}

	return pieces
}

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield Bitfield
	Peer     peer_discovery.Peer
	infoHash [20]byte
	peerID   [20]byte
}

func New(peer peer_discovery.Peer, torrent *bencode.TorrentType) (*Client, error) {
	dialer := net.Dialer{
		Timeout: connectionWaitFactor * time.Second,
	}
	peerAddress := peer.GetTCPAddress()
	conn, err := dialer.Dial("tcp", peerAddress)
	if err != nil {
		return nil, err
	}
	handshakeTimeout := 30 * time.Second
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	handshakeResponse, err := handshake.DoHandshake(conn, protocolIdentifier, torrent)
	conn.SetDeadline(time.Time{})
	if err != nil {
		return nil, errors.New("handshake failed: " + err.Error())
	}

	client := Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: make([]byte, 0),
		Peer:     peer,
		infoHash: torrent.InfoHash,
		peerID:   handshakeResponse.PeerID,
	}

	return &client, nil
}

func (client *Client) Read() (*message.Message, error) {
	return message.ReadMessage(client.Conn)
}

func (client *Client) SendRequest(requestIndex int, requestBegin int, requestLength int) error {
	req := message.CreateRequest(requestIndex, requestBegin, requestLength)
	client.Conn.SetWriteDeadline(time.Now().Add(connectionWaitFactor * time.Second * 10))
	_, err := client.Conn.Write(req.Serialize())
	client.Conn.SetWriteDeadline(time.Time{})
	return err
}

func (client *Client) SendHave(requestIndex int) error {
	req := message.CreateHave(requestIndex)
	client.Conn.SetWriteDeadline(time.Now().Add(connectionWaitFactor * time.Second * 10))
	_, err := client.Conn.Write(req.Serialize())
	client.Conn.SetWriteDeadline(time.Time{})
	return err
}
