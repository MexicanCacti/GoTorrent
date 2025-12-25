package networking

import (
	"GoTorrent/bencode"
	client2 "GoTorrent/client"
	"GoTorrent/message"
	"GoTorrent/peer_discovery"
	"GoTorrent/work"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type Work = work.Work
type WorkResults = work.WorkResults
type WorkProgress = work.WorkProgress

/*
const ConnectionWaitFactor = 10
const ProtocolLength = 19
const ProtocolIdentifier = "BitTorrent protocol"
*/

const requestSize = 16384 // 16kib block requests at a time
const waitTimeFactor = 30
const maxBacklog = 5
const maxRetries = 5

func ConstructWorkQueue(torrent *bencode.TorrentType) (chan *Work, chan *WorkResults) {
	workQueue := make(chan *Work, len(torrent.PieceHashes))
	results := make(chan *WorkResults)

	for index, hash := range torrent.PieceHashes {
		length := torrent.CalcPieceSize(index)
		workQueue <- &Work{Index: index, WorkHash: hash, Length: length}
	}

	return workQueue, results
}

func WritePieces(results chan *WorkResults, torrent *bencode.TorrentType, fileWriter *os.File) {
	fileWriter.Truncate(torrent.Length)

	piecesWritten := 0
	for piecesWritten < torrent.NumPieces {
		res := <-results
		offset := int64(res.PieceIndex) * torrent.PieceLength
		fileWriter.WriteAt(res.Buf, offset)
		piecesWritten += 1
	}
}

func ConnectToPeer(peer peer_discovery.Peer, torrent *bencode.TorrentType, wg *sync.WaitGroup, workQueue chan *Work, results chan *WorkResults) {
	defer wg.Done()

	client, err := client2.New(peer, torrent)
	if err != nil {
		log.Printf("could not create client: %v\n", err)
		return
	}

	defer func(Conn net.Conn) {
		err := Conn.Close()
		if err != nil {

		}
	}(client.Conn)

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

		default:
			log.Printf("Unexpected message type: %v\n", m.Name())
		}
	}

}

func startRequesting(client *client2.Client, workQueue chan *Work, results chan *WorkResults) {
	for assignedWork := range workQueue {
		if assignedWork.RetryCount > maxRetries {
			log.Printf(fmt.Sprintf("client: [%v]: used allocated retrys", client.Peer.IP))
		}

		if !client.Bitfield.HasPiece(assignedWork.Index) {
			assignedWork.RetryCount++
			workQueue <- assignedWork
			continue
		}

		buf, err := attemptPieceDownload(client, assignedWork)
		// Failed to download, client lied, set bitmap to 0, place work back in channel
		if err != nil {
			assignedWork.RetryCount++
			workQueue <- assignedWork
			client.Bitfield.ClearPiece(assignedWork.Index)
			if assignedWork.RetryCount > maxRetries {
				return
			}
			continue
		}

		// Verify
		err = compareHash(assignedWork, buf)
		if err != nil {
			log.Printf(err.Error())
			workQueue <- assignedWork
			client.Bitfield.ClearPiece(assignedWork.Index)
			if assignedWork.RetryCount > maxRetries {
				return
			}
			continue
		}
		_ = client.SendHave(assignedWork.Index)
		results <- &WorkResults{PieceIndex: assignedWork.Index, Buf: buf}
	}
}

func attemptPieceDownload(client *client2.Client, work *Work) ([]byte, error) {
	workProgress := WorkProgress{
		Index:      work.Index,
		Client:     client,
		Buf:        make([]byte, work.Length),
		Downloaded: 0,
		Requested:  0,
		Backlog:    0,
	}

	for workProgress.Downloaded < work.Length {

		// If we don't get it in 30 seconds assume we are not getting a response
		if !workProgress.Client.Choked {
			for workProgress.Backlog < maxBacklog && workProgress.Requested < work.Length {
				currentRequestSize := requestSize
				if work.Length-workProgress.Requested < currentRequestSize {
					currentRequestSize = work.Length - workProgress.Requested
				}
				err := client.SendRequest(work.Index, workProgress.Requested, currentRequestSize)
				if err != nil {
					return nil, err
				}
				workProgress.Requested += currentRequestSize
				workProgress.Backlog += 1
			}
		}
		client.Conn.SetReadDeadline(time.Now().Add(waitTimeFactor * time.Second))
		err := workProgress.ReadMessage()
		client.Conn.SetReadDeadline(time.Time{})
		if err != nil {
			return nil, err
		}
	}

	if workProgress.Downloaded != work.Length {
		return nil, fmt.Errorf("incomplete piece: %d/%d", workProgress.Downloaded, work.Length)
	}
	return workProgress.Buf, nil

}

func compareHash(work *Work, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], work.WorkHash[:]) {
		return errors.New(fmt.Sprintf("index %d hash mismatch", work.Index))
	}
	return nil
}
