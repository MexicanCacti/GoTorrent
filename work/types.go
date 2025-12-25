package work

import (
	"GoTorrent/client"
	"GoTorrent/message"
)

const bytesPerChunk = 20

type Work struct {
	Index      int
	WorkHash   [bytesPerChunk]byte
	Length     int
	RetryCount int
}

type WorkResults struct {
	PieceIndex int
	Buf        []byte
}

type WorkProgress struct {
	Index      int
	Client     *client.Client
	Buf        []byte
	Downloaded int
	Requested  int
	Backlog    int
}

func (workProgress *WorkProgress) ReadMessage() error {
	msg, _ := workProgress.Client.Read()
	// Keep alive
	if msg == nil {
		return nil
	}
	switch msg.ID {
	case message.MsgUnchoke:
		workProgress.Client.Choked = false
	case message.MsgChoke:
		workProgress.Client.Choked = true
	case message.MsgHave:
		index, _ := message.ParseHave(msg)
		workProgress.Client.Bitfield.SetPiece(index)
	case message.MsgPiece:
		dataAmount, _ := message.ParsePiece(workProgress.Index, workProgress.Buf, msg)
		workProgress.Downloaded += dataAmount
		workProgress.Backlog -= 1
	case message.MsgBitfield:
		workProgress.Client.Bitfield, _ = message.ParseBitfield(msg)
	}
	return nil
}
