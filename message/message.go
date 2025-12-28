package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const readWaitTimeFactor = 30

type messageID uint8

const (
	MsgChoke         messageID = 0
	MsgUnchoke       messageID = 1
	MsgInterested    messageID = 2
	MsgNotInterested messageID = 3
	MsgHave          messageID = 4
	MsgBitfield      messageID = 5
	MsgRequest       messageID = 6
	MsgPiece         messageID = 7
	MsgCancel        messageID = 8
)

type Message struct {
	ID      messageID
	Payload []byte
}

func (m *Message) Serialize() []byte {
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

func DeserializeMessage(buf []byte) *Message {
	if len(buf) == 0 {
		return nil
	}
	m := Message{
		ID: messageID(buf[0]),
	}
	if len(buf) > 1 {
		m.Payload = make([]byte, len(buf)-1)
		copy(m.Payload, buf[1:])
	}

	return &m
}

func SendInterested(conn net.Conn) error {
	message := Message{
		ID:      MsgInterested,
		Payload: make([]byte, 0),
	}
	_, err := conn.Write(message.Serialize())
	return err
}

func ReadMessage(conn net.Conn) (*Message, error) {
	lengthBuffer := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(readWaitTimeFactor * time.Second))
	_, err := io.ReadFull(conn, lengthBuffer)
	conn.SetReadDeadline(time.Time{})
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

	message := DeserializeMessage(messageBuf)
	return message, nil
}

func (m *Message) Name() string {
	if m == nil {
		return "KeepAlive"
	}

	switch m.ID {
	case MsgChoke:
		return "Choke"
	case MsgUnchoke:
		return "Unchoke"
	case MsgInterested:
		return "Interested"
	case MsgNotInterested:
		return "NotInterested"
	case MsgHave:
		return "Have"
	case MsgBitfield:
		return "Bitfield"
	case MsgRequest:
		return "Request"
	case MsgPiece:
		return "Piece"
	case MsgCancel:
		return "Cancel"
	}
	return fmt.Sprintf("Unknown Message ID: %d", m.ID)
}

func ParseHave(m *Message) (int, error) {
	if m.ID != MsgHave {
		return 0, errors.New(fmt.Sprintf("expected message ID: %d, got: %d", MsgHave, m.ID))
	}
	if len(m.Payload) != 4 {
		return 0, errors.New(fmt.Sprintf("expected payload length: %d, got: %d", 4, len(m.Payload)))
	}
	index := int(binary.BigEndian.Uint32(m.Payload))
	return index, nil
}

func ParsePiece(index int, buf []byte, m *Message) (int, error) {
	if m.ID != MsgPiece {
		return 0, errors.New(fmt.Sprintf("expected message ID: %d, got: %d", MsgPiece, m.ID))
	}
	if len(m.Payload) < 8 {
		return 0, errors.New(fmt.Sprintf("too short payload length: %d", len(m.Payload)))
	}
	parsedIndex := int(binary.BigEndian.Uint32(m.Payload[0:4]))
	if parsedIndex != index {
		return 0, errors.New(fmt.Sprintf("expected payload index: %d, got: %d", index, parsedIndex))
	}
	begin := int(binary.BigEndian.Uint32(m.Payload[4:8]))
	if begin < 0 || begin >= len(buf) {
		return 0, fmt.Errorf("begin offset out of bounds: %d (buf len %d)", begin, len(buf))
	}
	data := m.Payload[8:]
	if begin+len(data) > len(buf) {
		return 0, errors.New(fmt.Sprintf("data does not fit it buffer of len[%d], begin: %d, length: %d", len(buf), begin, len(data)))
	}
	copy(buf[begin:begin+len(data)], data)
	return len(data), nil
}

func ParseBitfield(m *Message) ([]byte, error) {
	if m.ID != MsgBitfield {
		return nil, errors.New(fmt.Sprintf("expected message ID: %d, got: %d", MsgBitfield, m.ID))
	}

	bitfieldLength := len(m.Payload)
	bitfield := make([]byte, bitfieldLength)
	copy(bitfield, m.Payload)
	return bitfield, nil
}

func CreateRequest(requestIndex int, requestBegin int, requestLength int) *Message {
	payload := make([]byte, 12) // each int should take up 4 bytes
	binary.BigEndian.PutUint32(payload[0:4], uint32(requestIndex))
	binary.BigEndian.PutUint32(payload[4:8], uint32(requestBegin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(requestLength))
	return &Message{
		ID:      MsgRequest,
		Payload: payload,
	}
}

func CreateHave(requestIndex int) *Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(requestIndex))
	return &Message{
		ID:      MsgHave,
		Payload: payload,
	}
}
