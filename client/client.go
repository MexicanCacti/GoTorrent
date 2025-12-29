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

const connectionWaitFactor = 5
const protocolIdentifier = "BitTorrent protocol"

type Bitfield []byte // 0 indexed... 0b110, piece 2 is missing, 0b011, piece 0 is missing, big endian
// Size: math.ceil(numPieces / 8)
// Spare: 8 * Size - numPieces
// Client MAY send bitfield, MUST be first msg after handshake

func (bitfield *Bitfield) HasPiece(index int) bool {
	if index < 0 {
		return false
	}
	byteIndex := index / 8
	byteOffset := index % 8
	if len(*bitfield) <= byteIndex {
		return false
	}
	return (*bitfield)[byteIndex]>>(7-byteOffset)&1 != 0
}

func (bitfield *Bitfield) SetPiece(index int) {
	if index < 0 {
		return
	}
	byteIndex := index / 8
	byteOffset := index % 8
	if byteIndex >= len(*bitfield) {
		newLen := byteIndex + 1
		newBitfield := make(Bitfield, newLen)
		copy(newBitfield, *bitfield)
		*bitfield = newBitfield
	}

	(*bitfield)[byteIndex] |= 1 << (7 - byteOffset)
}

func (bitfield *Bitfield) ClearPiece(index int) {
	byteIndex := index / 8
	byteOffset := index % 8
	(*bitfield)[byteIndex] &= ^(1 << (7 - byteOffset))
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

func getBitfield(conn net.Conn) (Bitfield, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{})

	msg, err := message.ReadMessage(conn)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errors.New("message is nil but should be bitfield")
	}
	if msg.ID != message.MsgBitfield {
		return nil, errors.New("invalid message id received")
	}

	return msg.Payload, nil
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
	conn, err := net.Dial("tcp", peer.GetTCPAddress())
	if err != nil {
		return nil, err
	}

	handshakeResponse, err := handshake.DoHandshake(conn, protocolIdentifier, torrent)
	if err != nil {
		conn.Close()
		return nil, errors.New("handshake failed: " + err.Error())
	}

	bitfield, err := getBitfield(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	client := Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: bitfield,
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
	_, err := client.Conn.Write(req.Serialize())
	return err
}

func (client *Client) SendHave(requestIndex int) error {
	req := message.CreateHave(requestIndex)
	_, err := client.Conn.Write(req.Serialize())
	return err
}

func (client *Client) SendUnchoke() error {
	msg := message.CreateUnchoke()
	_, err := client.Conn.Write(msg.Serialize())
	return err
}
