package main

import (
	"GoTorrent/bencode"
	"GoTorrent/networking"
	"GoTorrent/peer_discovery"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const portNum int = 7777
const numWriters int = 3

func GeneratePeerID() ([20]byte, error) {
	var peerID [20]byte

	copy(peerID[0:8], []byte("-GT0001-"))

	_, err := rand.Read(peerID[8:])
	if err != nil {
		return peerID, err
	}

	return peerID, nil
}

func main() {
	peerID, err := GeneratePeerID()
	if err != nil {
		log.Fatal(err)
	}

	torrentPath, err := bencode.PickTorrent()
	if err != nil {
		log.Fatal(err)
	}

	fileReader, err := os.Open(torrentPath)
	if err != nil {
		log.Fatal(err)
	}

	torrent, err := bencode.ParseTorrent(fileReader, torrentPath)
	if err != nil {
		log.Fatal(err)
	}

	fileReader.Close()

	folder, err := bencode.PickDownloadPath()
	if err != nil {
		log.Fatal(err)
	}
	savePath := filepath.Join(folder, torrent.Name)

	torrent.PeerID = peerID
	peerList, err := peer_discovery.GetPeers(&torrent, peerID, uint16(portNum))
	if err != nil {
		log.Fatal(err)
	}

	workQueue, results := networking.ConstructWorkQueue(&torrent)
	openFiles, err := bencode.OpenFiles(&torrent, savePath)

	if err != nil {
		log.Fatal(err)
	}

	var writeGroup sync.WaitGroup
	var writtenPieces int64 = 0
	var totalPieces = int64(torrent.NumPieces)
	for i := 0; i < numWriters; i++ {
		writeGroup.Add(1)
		go networking.WritePieces(results, &torrent, openFiles, &writeGroup, &writtenPieces, &totalPieces)
	}

	var downloadedPieces int64 = 0
	log.Printf("Total Pieces: %d\n", totalPieces)
	go func() {
		for {
			if atomic.LoadInt64(&writtenPieces) == totalPieces {
				close(workQueue)
				return
			}
		}
	}()

	var torrentGroup sync.WaitGroup
	for i, peer := range *peerList {
		fmt.Printf("Peer [%v]: IP: %v, Port: %v\n", i, peer.IP, peer.Port)
		torrentGroup.Add(1)
		go networking.ConnectToPeer(peer, &torrent, &torrentGroup, workQueue, results, &downloadedPieces, &totalPieces)
	}
	torrentGroup.Wait()
	log.Println("TORRENT GROUP DONE")
	writeGroup.Wait()
	log.Println("WRITE GROUP DONE")
	close(results)

	for _, file := range openFiles {
		file.Close()
	}
}
