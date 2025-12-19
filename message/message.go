package message

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

type messageID uint8

const (
	messageChoke         messageID = 0
	messageUnchoke       messageID = 1
	messageInterested    messageID = 2
	messageNotInterested messageID = 3
	messageHave          messageID = 4
	messageBitfield      messageID = 5
	messageRequest       messageID = 6
	messagePiece         messageID = 7
	messageCancel        messageID = 8
)

type Message struct {
	ID      messageID
	Payload []byte
}

func (m *Message) serialize() []byte {
	if m == nil {
		return make([]byte, 4)
	} // Length is a 32-bit integer, always 4 bytes
	messageLength := uint32(len(m.Payload) + 1) // +1 for ID
	buf := make([]byte, 4+messageLength)
	binary.BigEndian.PutUint32(buf[0:4], messageLength)
	buf[4] = byte(m.ID)
	copy(buf[5:], m.Payload)
	return buf
}

func deserializeMessage(buf []byte) *Message {
	m := Message{}
	messageLength := uint32(0)
	if len(buf) == 4 {
		messageLength += binary.BigEndian.Uint32(buf[0:4])
	}

	if messageLength >= 4 {
		m.ID = messageID(buf[0])
	}

	if messageLength >= 5 {
		copy(m.Payload, buf[4:])
	}

	return &m
}

func SendUnchoke(conn net.Conn) error {
	message := Message{
		ID:      messageUnchoke,
		Payload: make([]byte, 0),
	}
	_, err := conn.Write(message.serialize())

	return err
}

func SendInterested(conn net.Conn) error {
	message := Message{
		ID:      messageInterested,
		Payload: make([]byte, 0),
	}
	_, err := conn.Write(message.serialize())
	return err
}

func ReadMessage(conn net.Conn) (*Message, error) {
	lengthBuffer := make([]byte, 4)
	_, err := io.ReadFull(conn, lengthBuffer)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuffer)

	if length == 0 {
		return nil, nil
	}

	messageBuf := make([]byte, length)
	_, err = io.ReadFull(conn, messageBuf)
	if err != nil {
		return nil, err
	}

	message := Message{
		ID:      messageID(messageBuf[0]),
		Payload: messageBuf[1:],
	}
	return &message, nil
}

func (m *Message) Name() string {
	if m == nil {
		return "KeepAlive"
	}

	switch m.ID {
	case messageChoke:
		return "Choke"
	case messageUnchoke:
		return "Unchoke"
	case messageInterested:
		return "Interested"
	case messageNotInterested:
		return "NotInterested"
	case messageHave:
		return "Have"
	case messageBitfield:
		return "Bitfield"
	case messageRequest:
		return "Request"
	case messagePiece:
		return "Piece"
	case messageCancel:
		return "Cancel"
	}
	return fmt.Sprintf("Unknown Message ID: %d", m.ID)
}
