package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

type messageID uint8

const (
	MessageChoke         messageID = 0
	MessageUnchoke       messageID = 1
	MessageInterested    messageID = 2
	MessageNotInterested messageID = 3
	MessageHave          messageID = 4
	MessageBitfield      messageID = 5
	MessageRequest       messageID = 6
	MessagePiece         messageID = 7
	MessageCancel        messageID = 8
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
		ID:      MessageInterested,
		Payload: make([]byte, 0),
	}
	_, err := conn.Write(message.Serialize())
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

	message := DeserializeMessage(messageBuf)
	return message, nil
}

func (m *Message) Name() string {
	if m == nil {
		return "KeepAlive"
	}

	switch m.ID {
	case MessageChoke:
		return "Choke"
	case MessageUnchoke:
		return "Unchoke"
	case MessageInterested:
		return "Interested"
	case MessageNotInterested:
		return "NotInterested"
	case MessageHave:
		return "Have"
	case MessageBitfield:
		return "Bitfield"
	case MessageRequest:
		return "Request"
	case MessagePiece:
		return "Piece"
	case MessageCancel:
		return "Cancel"
	}
	return fmt.Sprintf("Unknown Message ID: %d", m.ID)
}

func ParseHave(m *Message) (int, error) {
	if m.ID != MessageHave {
		return 0, errors.New(fmt.Sprintf("expected message ID: %d, got: %d", MessageHave, m.ID))
	}
	if len(m.Payload) != 4 {
		return 0, errors.New(fmt.Sprintf("expected payload length: %d, got: %d", 4, len(m.Payload)))
	}
	index := int(binary.BigEndian.Uint32(m.Payload))
	return index, nil
}

func ParsePiece(index int, buf []byte, m *Message) (int, error) {
	if m.ID != MessagePiece {
		return 0, errors.New(fmt.Sprintf("expected message ID: %d, got: %d", MessagePiece, m.ID))
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
	if m.ID != MessageBitfield {
		return nil, errors.New(fmt.Sprintf("expected message ID: %d, got: %d", MessageBitfield, m.ID))
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
		ID:      MessageRequest,
		Payload: payload,
	}
}

func CreateHave(requestIndex int) *Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(requestIndex))
	return &Message{
		ID:      MessageHave,
		Payload: payload,
	}
}
