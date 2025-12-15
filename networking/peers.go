package networking

import (
	"GoTorrent/torrentstruct"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackpal/bencode-go"
)

type trackerResponse struct {
	Interval int64  `bencode:"interval"`
	peers    string `bencode:"peers"`
}

type Peer struct {
	IP   net.IP
	Port uint16
}

func GetPeers(t *torrentstruct.TorrentType, peerID [20]byte, port uint16) (*[]Peer, error) {

	protocol, err := url.Parse(t.Announce)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	switch protocol.Scheme {
	case "http", "https":
		return buildHTTP(t, peerID, port)
	case "udp":
		return buildUDP(t, peerID)
	default:
		return nil, fmt.Errorf("unsupported protocol scheme %s", protocol.Scheme)
	}
}

func buildHTTP(t *torrentstruct.TorrentType, peerID [20]byte, port uint16) (*[]Peer, error) {
	base, err := url.Parse(t.Announce)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"info_hash":  []string{string(t.InfoHash[:])},
		"peer_id":    []string{string(peerID[:])},
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(int(t.Length))},
	}

	base.RawQuery = params.Encode()
	return httpQueryTracker(base.String())

}

func httpQueryTracker(queryString string) (*[]Peer, error) {
	resp, err := http.Get(queryString)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	trackerResponse := trackerResponse{}

	err = bencode.Unmarshal(bytes.NewReader(respBody), &trackerResponse)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return extractPeers(&trackerResponse)
}

/*
See: https://xbtt.sourceforge.net/udp_tracker_protocol.html
for formats of inputs/outputs
*/
func buildUDP(t *torrentstruct.TorrentType, peerID [20]byte, port uint16) (*[]Peer, error) {
	conn, err := net.Dial("udp", t.Announce)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	defer conn.Close()
	transactionID := rand.Uint32()

	buf := new(bytes.Buffer)
	protocolID := uint64(0x41727101980) //Note magic costant for udp tracker
	action := uint32(0)

	binary.Write(buf, binary.BigEndian, protocolID)
	binary.Write(buf, binary.BigEndian, action)
	binary.Write(buf, binary.BigEndian, transactionID)
	connectionID, err := getConnectionID(conn, transactionID)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	base, err := url.Parse(t.Announce)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"connection_id":  []string{string(connectionID)},
		"action":         []string{string("1")},
		"transaction_id": []string{string(transactionID)},
		"info_hash":      []string{string(t.InfoHash[:])},
		"peer_id":        []string{string(peerID[:])},
		"downloaded":     []string{"0"},
		"left":           []string{strconv.Itoa(int(t.Length))},
		"uploaded":       []string{"0"},
		"event":          []string{string("0")},
		"IP address":     []string{string("0")},
		"key":            []string{string("0")},
		"num_want":       []string{string("-1")},
		"port":           []string{strconv.Itoa(int(port))},
	}
	base.RawQuery = params.Encode()

}

func getConnectionID(conn net.Conn, transactionID uint32) (uint64, error) {
	buf := new(bytes.Buffer)
	protocolID := uint64(0x41727101980) //Note magic costant for udp tracker
	action := uint32(0)

	binary.Write(buf, binary.BigEndian, protocolID)
	binary.Write(buf, binary.BigEndian, action)
	binary.Write(buf, binary.BigEndian, transactionID)

	return transmitConnectionRequest(conn, buf, transactionID)
}

func transmitConnectionRequest(conn net.Conn, buf *bytes.Buffer, transactionID uint32) (uint64, error) {
	var err error
	_, err = conn.Write(buf.Bytes())
	if err != nil {
		log.Fatal(err)
		return 0, err
	}

	resp := make([]byte, 16)
	err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		log.Fatal(err)
		return 0, err
	}
	n, err := conn.Read(resp)
	if err != nil {
		log.Fatal(err)
		return 0, err
	}
	if n < 16 {
		return 0, fmt.Errorf("invalid response length: %d", n)
	}

	respBuf := bytes.NewBuffer(resp)
	var respAction uint32
	var respTransactionID uint32
	var respConnectionID uint64

	binary.Read(respBuf, binary.BigEndian, &respAction)
	binary.Read(respBuf, binary.BigEndian, &respTransactionID)
	binary.Read(respBuf, binary.BigEndian, &respConnectionID)

	if respAction != 0 {
		return 0, fmt.Errorf("invalid response action: %d", respAction)
	}
	if respTransactionID != transactionID {
		return 0, fmt.Errorf("transaction ID mismatch: %d", respTransactionID)
	}

	return respConnectionID, nil

}

func extractPeers(tResp *trackerResponse) (*[]Peer, error) {
	const peerSize = 6 // 4 IP, 2 Port
	numPeers := len(tResp.peers) / peerSize
	if len(tResp.peers)%peerSize != 0 {
		err := fmt.Errorf("Malformed peers received from tracker")
		return nil, err
	}
	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {

	}
	return &peers, nil
}
