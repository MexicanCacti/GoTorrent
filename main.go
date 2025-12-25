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
)

const portNum int = 7777

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

	torrent.PeerID = [20]byte(peerID)
	peerList, err := peer_discovery.GetPeers(&torrent, [20]byte(peerID), uint16(portNum))
	if err != nil {
		log.Fatal(err)
	}

	workQueue, results := networking.ConstructWorkQueue(&torrent)
	fileWriter, err := os.Create(savePath)
	if err != nil {
		log.Fatal(err)
	}
	defer fileWriter.Close()
	go networking.WritePieces(results, &torrent, fileWriter)

	var wg sync.WaitGroup
	for i, peer := range *peerList {
		fmt.Printf("Peer [%v]: IP: %v, Port: %v\n", i, peer.IP, peer.Port)
		wg.Add(1)
		go networking.ConnectToPeer(peer, &torrent, &wg, workQueue, results)
	}
	wg.Wait()
}
