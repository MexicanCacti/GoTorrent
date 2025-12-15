package main

import (
	"GoTorrent/networking"
	"GoTorrent/torrentstruct"
	"fmt"
	"log"
	"os"
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
	fmt.Println(torrent)

	peerList, err := networking.GetPeers(&torrent, [20]byte(peerID), uint16(portNum))

	for i, peer := range *peerList {
		fmt.Printf("Peer [%i]: IP: %v, Port: %v\n", i, peer.IP, peer.Port)
	}
}
