package networking

import (
	"GoTorrent/message"
	"GoTorrent/torrentstruct"
	"log"
	"sync"
)

const ConnectionWaitFactor = 10
const ProtocolLength = 19
const ProtocolIdentifier = "BitTorrent protocol"

const bytesPerChunk = 20

type Work struct {
	Index    int
	WorkHash [bytesPerChunk]byte
	Length   int
}

type WorkResults struct {
	PieceIndex     int
	PieceStartByte int
	PieceLength    int
	PieceHash      [bytesPerChunk]byte
}

func ConstructWorkQueue(torrent *torrentstruct.TorrentType) (chan *Work, chan *WorkResults) {
	workQueue := make(chan *Work, len(torrent.PieceHashes))
	results := make(chan *WorkResults)

	for index, hash := range torrent.PieceHashes {
		length := torrent.CalcPieceSize(index)
		workQueue <- &Work{index, hash, length}
	}

	return workQueue, results
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
			// Now begin requesting
			return

		case "Choke":
			client.Choked = true
			log.Printf("Choked\n")
			return

		}
	}

}
