package networking

import (
	"GoTorrent/message"
	"GoTorrent/torrentstruct"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

const ConnectionWaitFactor = 10
const ProtocolLength = 19
const ProtocolIdentifier = "BitTorrent protocol"

const bytesPerChunk = 20
const requestSize = 16384 // 16kib block requests at a time
const waitTimeFactor = 30
const maxBacklog = 5
const maxRetries = 5

type Work struct {
	Index      int
	WorkHash   [bytesPerChunk]byte
	Length     int
	RetryCount int
}

type WorkResults struct {
	PieceIndex int
	buf        []byte
}

type WorkProgress struct {
	index      int
	client     *Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

func (workProgress *WorkProgress) readMessage() error {
	msg, _ := workProgress.client.Read()
	// Keep alive
	if msg == nil {
		return nil
	}
	switch msg.ID {
	case message.MessageUnchoke:
		workProgress.client.Choked = false
	case message.MessageChoke:
		workProgress.client.Choked = true
	case message.MessageHave:
		index, _ := message.ParseHave(msg)
		workProgress.client.Bitfield.setPiece(index)
	case message.MessagePiece:
		dataAmount, _ := message.ParsePiece(workProgress.index, workProgress.buf, msg)
		workProgress.downloaded += dataAmount
		workProgress.backlog -= 1
	case message.MessageBitfield:
		workProgress.client.Bitfield, _ = message.ParseBitfield(msg)
	}
	return nil
}

func ConstructWorkQueue(torrent *torrentstruct.TorrentType) (chan *Work, chan *WorkResults) {
	workQueue := make(chan *Work, len(torrent.PieceHashes))
	results := make(chan *WorkResults)

	for index, hash := range torrent.PieceHashes {
		length := torrent.CalcPieceSize(index)
		workQueue <- &Work{index, hash, length, 0}
	}

	return workQueue, results
}

func WritePieces(results chan *WorkResults, torrent *torrentstruct.TorrentType, fileWriter *os.File) {
	fileWriter.Truncate(torrent.Length)

	piecesWritten := 0
	for piecesWritten < torrent.NumPieces {
		res := <-results
		offset := int64(res.PieceIndex) * torrent.PieceLength
		fileWriter.WriteAt(res.buf, offset)
		piecesWritten += 1
	}
}

func ConnectToPeer(peer Peer, torrent *torrentstruct.TorrentType, wg *sync.WaitGroup, workQueue chan *Work, results chan *WorkResults) {
	defer wg.Done()

	client, err := New(peer, torrent)
	if err != nil {
		log.Printf("could not create client: %v\n", err)
		return
	}

	defer client.Conn.Close()

	//fmt.Printf("IP: %v | Port: %v | ID: %v\n", peer.IP, peer.Port, client.peerID)

	err = message.SendInterested(client.Conn)
	if err != nil {
		return
	}

	for {
		if !client.Choked && len(client.Bitfield) > 0 {
			startRequesting(client, workQueue, results)
		}
		m, err := message.ReadMessage(client.Conn)
		if err != nil {
			log.Printf("could not read message: %v\n", err)
			return
		}

		if m == nil {
			continue // keep-alive
		}

		switch m.Name() {
		case "Unchoke":
			client.Choked = false
			log.Printf("Unchoked\n")

		case "Choke":
			client.Choked = true
			log.Printf("Choked\n")

		case "Bitfield":
			client.Bitfield, _ = message.ParseBitfield(m)
			log.Printf("Bitfield\n")
		}
	}

}

func startRequesting(client *Client, workQueue chan *Work, results chan *WorkResults) {
	for work := range workQueue {
		if !client.Bitfield.hasPiece(work.Index) {
			work.RetryCount++
			workQueue <- work
			continue
		}

		buf, err := attemptPieceDownload(client, work)
		// Failed to download, client lied, set bitmap to 0, place work back in channel
		if err != nil {
			work.RetryCount++
			workQueue <- work
			client.Bitfield.clearPiece(work.Index)
			if work.RetryCount > maxRetries {
				return
			}
			continue
		}

		// Verify
		err = compareHash(work, buf)
		if err != nil {
			log.Printf(err.Error())
			workQueue <- work
			client.Bitfield.clearPiece(work.Index)
			if work.RetryCount > maxRetries {
				return
			}
			continue
		}
		log.Printf("finished downloading piece %d\n", work.Index)
		client.SendHave(work.Index)
		results <- &WorkResults{work.Index, buf}
	}
}

func attemptPieceDownload(client *Client, work *Work) ([]byte, error) {
	workProgress := WorkProgress{
		index:      work.Index,
		client:     client,
		buf:        make([]byte, work.Length),
		downloaded: 0,
		requested:  0,
		backlog:    0,
	}

	for workProgress.downloaded < work.Length {
		client.Conn.SetDeadline(time.Now().Add(waitTimeFactor * time.Second))
		// If we don't get it in 30 seconds assume we are not getting a response
		defer client.Conn.SetDeadline(time.Time{})
		if !workProgress.client.Choked {
			for workProgress.backlog < maxBacklog && workProgress.requested < work.Length {
				currentRequestSize := requestSize
				if work.Length-workProgress.requested < currentRequestSize {
					currentRequestSize = work.Length - workProgress.requested
				}
				err := client.SendRequest(work.Index, workProgress.requested, currentRequestSize)
				if err != nil {
					return nil, err
				}
				workProgress.requested += currentRequestSize
				workProgress.backlog += 1
			}
		}
		err := workProgress.readMessage()
		if err != nil {
			return nil, err
		}
	}

	if workProgress.downloaded != work.Length {
		return nil, fmt.Errorf("incomplete piece: %d/%d", workProgress.downloaded, work.Length)
	}
	return workProgress.buf, nil

}

func compareHash(work *Work, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], work.WorkHash[:]) {
		return errors.New(fmt.Sprintf("index %d hash mismatch", work.Index))
	}
	return nil
}
