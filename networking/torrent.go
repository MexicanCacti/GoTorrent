package networking

import (
	"GoTorrent/bencode"
	clientImport "GoTorrent/client"
	"GoTorrent/message"
	"GoTorrent/peer_discovery"
	"GoTorrent/work"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Work = work.Work
type WorkResults = work.Results
type WorkProgress = work.Progress

/*
const ConnectionWaitFactor = 10
const ProtocolLength = 19
const ProtocolIdentifier = "BitTorrent protocol"
*/

const requestSize = 16384 // 16kib block requests at a time
const maxBacklog = 5
const clientCreationRetries = 1
const clientCreationTimeout = 5
const downloadTimeoutFactor = 30
const workRetrievalTimeout = 100

func ConstructWorkQueue(torrent *bencode.TorrentType) (chan *Work, chan *WorkResults) {
	workQueue := make(chan *Work, len(torrent.PieceHashes))
	results := make(chan *WorkResults)

	for index, hash := range torrent.PieceHashes {
		length := torrent.CalcPieceSize(index)
		workQueue <- &Work{Index: index, WorkHash: hash, Length: length}
	}

	return workQueue, results
}

func WritePieces(results chan *WorkResults, torrent *bencode.TorrentType, openFiles []*os.File, wg *sync.WaitGroup, writtenPieces *int64, totalPieces *int64) {
	defer wg.Done()
	log.Printf("NumFiles: %d", len(openFiles))
	for {
		select {
		case res := <-results:
			pieceStart := int64(res.PieceIndex) * torrent.PieceLength
			pieceEnd := pieceStart + int64(len(res.Buf))

			for i, f := range torrent.Files {
				fileStart := f.Offset
				fileEnd := f.Offset + f.Length
				if pieceEnd <= fileStart || pieceStart >= fileEnd {
					continue // Piece does not write to this file
				}

				// Find what part of the piece belongs to this file
				writeStart := max(pieceStart, fileStart)
				writeEnd := min(pieceEnd, fileEnd)

				bufOffset := writeStart - pieceStart
				fileOffset := writeStart - fileStart

				_, err := openFiles[i].WriteAt(res.Buf[bufOffset:bufOffset+writeEnd-writeStart], fileOffset)
				if err != nil {
					results <- res
					continue
				}
				piecesWritten := atomic.AddInt64(writtenPieces, 1)
				percent := (float64(piecesWritten) / float64(torrent.NumPieces)) * 100
				log.Printf("[%0.2f%%]: Wrote piece [%d]", percent, res.PieceIndex)

			}
		case <-time.After(workRetrievalTimeout * time.Second * 5):
			log.Printf("write retrieval timeout")
			return
		}

	}
}

func ConnectToPeer(peer peer_discovery.Peer, torrent *bencode.TorrentType, wg *sync.WaitGroup, workQueue chan *Work, results chan *WorkResults, completedPieces *int64, totalPieces *int64) {
	defer wg.Done()
	var client *clientImport.Client
	var err error
	for i := 0; i < clientCreationRetries; i++ {
		client, err = clientImport.New(peer, torrent)
		if err != nil {
			log.Printf("retry create client [%v]\n", err)
			time.Sleep(clientCreationTimeout * time.Second)
			continue
		}
		break
	}
	if err != nil {
		log.Printf("failed to create client [%v]\n", err)
		return
	}

	log.Printf("created client with peer [%v]\n", peer.GetTCPAddress())

	//fmt.Printf("IP: %v | Port: %v | ID: %v\n", peer.IP, peer.Port, client.peerID)

	err = client.SendUnchoke()
	if err != nil {
		log.Printf("failed to send unchoke [%v]\n", err)
		return
	}
	err = message.SendInterested(client.Conn)

	if err != nil {
		log.Printf("failed to send interested [%v]\n", err)
		return
	}

	for work := range workQueue {
		if atomic.LoadInt64(completedPieces) == *totalPieces {
			return
		}

		if !client.Bitfield.HasPiece(work.Index) {
			workQueue <- work
			continue
		}

		buf, err := attemptPieceDownload(client, work)
		if err != nil {
			log.Printf("failed to download piece [%d], %v\n", work.Index, err)
			workQueue <- work
			return
		}

		err = compareHash(work, buf)
		if err != nil {
			log.Printf("failed hash check [%d]\n", work.Index)
			workQueue <- work
			continue
		}

		client.SendHave(work.Index)
		results <- &WorkResults{PieceIndex: work.Index, Buf: buf}
		atomic.AddInt64(completedPieces, 1)
	}
}

func attemptPieceDownload(client *clientImport.Client, work *Work) ([]byte, error) {
	workProgress := WorkProgress{
		Index:      work.Index,
		Client:     client,
		Buf:        make([]byte, work.Length),
		Downloaded: 0,
		Requested:  0,
		Backlog:    0,
	}

	// If we don't get it in 30 seconds assume we are not getting a response
	client.Conn.SetDeadline(time.Now().Add(downloadTimeoutFactor * time.Second))
	defer client.Conn.SetDeadline(time.Time{})
	for workProgress.Downloaded < work.Length {
		if !workProgress.Client.Choked {
			for workProgress.Backlog < maxBacklog && workProgress.Requested < work.Length {
				currentRequestSize := requestSize
				if work.Length-workProgress.Requested < currentRequestSize {
					currentRequestSize = work.Length - workProgress.Requested
				}
				err := client.SendRequest(work.Index, workProgress.Requested, currentRequestSize)
				if err != nil {
					return nil, errors.New(fmt.Sprintf("failed to send request [%d], [%v]\n", work.Index, err))
				}
				workProgress.Requested += currentRequestSize
				workProgress.Backlog += 1
			}
		}
		err := workProgress.ReadMessage()
		if err != nil {
			return nil, errors.New(fmt.Sprintf("failed to read response [%d], [%v]\n", work.Index, err))
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
