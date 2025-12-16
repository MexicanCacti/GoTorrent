package main

import (
	"GoTorrent/networking"
	"GoTorrent/torrentstruct"
	"fmt"
	"log"
	"os"
	"sync"
)

const portNum int = 7777

func main() {
	var peerID = make([]byte, 20)

	torrentPath, err := torrentstruct.PickTorrent()
	if err != nil {
		log.Fatal(err)
	}

	fileReader, err := os.Open(torrentPath)
	if err != nil {
		log.Fatal(err)
	}

	torrent, err := torrentstruct.ParseTorrent(fileReader, torrentPath)
	if err != nil {
		log.Fatal(err)
	}

	torrent.PeerID = [20]byte(peerID)

	peerList, err := networking.GetPeers(&torrent, [20]byte(peerID), uint16(portNum))
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	for i, peer := range *peerList {
		fmt.Printf("Peer [%v]: IP: %v, Port: %v\n", i, peer.IP, peer.Port)
		wg.Add(1)
		go networking.ConnectToPeer(peer, &torrent, &wg)
	}

	wg.Wait()
}
