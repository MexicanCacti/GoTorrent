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

type Results struct {
	PieceIndex int
	Buf        []byte
}

type Progress struct {
	Index      int
	Client     *client.Client
	Buf        []byte
	Downloaded int
	Requested  int
	Backlog    int
}

func (workProgress *Progress) ReadMessage() error {
	msg, err := workProgress.Client.Read()
	if err != nil {
		return err
	}
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
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		workProgress.Client.Bitfield.SetPiece(index)
	case message.MsgPiece:
		dataAmount, err := message.ParsePiece(workProgress.Index, workProgress.Buf, msg)
		if err != nil {
			return err
		}
		workProgress.Downloaded += dataAmount
		workProgress.Backlog -= 1
	case message.MsgBitfield:
		workProgress.Client.Bitfield, err = message.ParseBitfield(msg)
		if err != nil {
			return err
		}
	}
	return nil
}
