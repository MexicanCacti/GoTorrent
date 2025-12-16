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

// NOTE: Create NEW() function to init default values
type udpResponse struct {
	Interval uint64 `bencode:"interval"`
	Peers    []byte `bencode:"peers"`
}

type httpResponse struct {
	Complete   uint64 `bencode:"complete"`
	Incomplete uint64 `bencode:"incomplete"`
	Interval   uint64 `bencode:"interval"`
	Peers      []Peer `bencode:"peers"`
}

type Peer struct {
	IP   string   `bencode:"ip"`
	Port uint16   `bencode:"port"`
	ID   [20]byte `bencode:"peer id"`
}

func (p *Peer) GetTCPAddress() string {
	return net.JoinHostPort(
		p.IP,
		strconv.Itoa(int(p.Port)),
	)
}

func GetPeers(t *torrentstruct.TorrentType, peerID [20]byte, port uint16) (*[]Peer, error) {
	protocol, err := url.Parse(t.Announce)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	fmt.Println(protocol.Scheme)
	switch protocol.Scheme {
	case "http", "https":
		return buildHTTP(t, peerID, port)
	case "udp":
		return buildUDP(t, peerID, port)
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
		"left":       []string{"0"},
	}

	base.RawQuery = params.Encode()
	return httpQueryTracker(base.String())

}

func httpQueryTracker(queryString string) (*[]Peer, error) {
	resp, err := http.Get(queryString)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println(err)
		}
	}(resp.Body)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	httpResponse := httpResponse{}

	err = bencode.Unmarshal(bytes.NewReader(respBody), &httpResponse)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return httpExtractPeers(&httpResponse)
}

/*
See: https://xbtt.sourceforge.net/udp_tracker_protocol.html
for formats of inputs/outputs
*/
func buildUDP(t *torrentstruct.TorrentType, peerID [20]byte, port uint16) (*[]Peer, error) {
	// Dial tracker
	u, err := url.Parse(t.Announce)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	conn, err := net.Dial("udp", u.Host)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	defer conn.Close()
	transactionID := rand.Uint32()

	// Connect input
	buf := new(bytes.Buffer)
	protocolID := uint64(0x41727101980) //Note magic costant for udp tracker
	action := uint32(0)

	binary.Write(buf, binary.BigEndian, protocolID)
	binary.Write(buf, binary.BigEndian, action)
	binary.Write(buf, binary.BigEndian, transactionID)

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	resp := make([]byte, 16)
	err = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err != nil {
		log.Fatal(err)
	}

	// Connect output
	n, err := conn.Read(resp)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	if n < 16 {
		return nil, fmt.Errorf("invalid response length: %d", n)
	}

	respBuf := bytes.NewBuffer(resp)
	var respAction uint32
	var respTransactionID uint32
	var respConnectionID uint64

	binary.Read(respBuf, binary.BigEndian, &respAction)
	binary.Read(respBuf, binary.BigEndian, &respTransactionID)
	binary.Read(respBuf, binary.BigEndian, &respConnectionID)

	if respAction != 0 {
		return nil, fmt.Errorf("invalid response action %d", respAction)
	}
	if respTransactionID != transactionID {
		return nil, fmt.Errorf("invalid transaction ID %d", respTransactionID)
	}

	transactionID = rand.Uint32()
	// Announce input
	buf = new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, respConnectionID)
	binary.Write(buf, binary.BigEndian, uint32(1)) // action
	binary.Write(buf, binary.BigEndian, transactionID)
	buf.Write(t.InfoHash[:])
	buf.Write(peerID[:])
	binary.Write(buf, binary.BigEndian, uint64(0))        // downloaded
	binary.Write(buf, binary.BigEndian, uint64(t.Length)) // left
	binary.Write(buf, binary.BigEndian, uint64(0))        // uploaded
	binary.Write(buf, binary.BigEndian, uint32(0))        // event
	binary.Write(buf, binary.BigEndian, uint32(0))        // IP address
	binary.Write(buf, binary.BigEndian, rand.Uint32())    // key
	binary.Write(buf, binary.BigEndian, int32(-1))        // num_want
	binary.Write(buf, binary.BigEndian, port)

	_, err = conn.Write(buf.Bytes())

	// Announce output
	announceBuf := make([]byte, 1500) // Hopefully good enough, 1472 theoretical max message size
	n, err = conn.Read(announceBuf)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	announceResp := announceBuf[:n]

	fmt.Printf("Size of resp: %v", n)
	var announceAction = binary.BigEndian.Uint32(announceResp[0:4])
	var announceTransactionID = binary.BigEndian.Uint32(announceResp[4:8])

	if announceAction != 1 {
		return nil, fmt.Errorf("invalid announce action %d", announceAction)
	}

	if announceTransactionID != transactionID {
		return nil, fmt.Errorf("invalid transaction ID %d", announceTransactionID)
	}

	var announceInterval = binary.BigEndian.Uint32(announceResp[8:12])
	var announceLeechers = binary.BigEndian.Uint32(announceResp[12:16])
	var announceSeeders = binary.BigEndian.Uint32(announceResp[16:20])

	fmt.Printf("\nLeechers : %v |\t", announceLeechers)
	fmt.Printf("Seeders: %v\n", announceSeeders)
	trackerResponse := udpResponse{}
	trackerResponse.Interval = uint64(announceInterval)
	trackerResponse.Peers = announceResp[20:]

	return udpExtractPeers(&trackerResponse)

}

func httpExtractPeers(hResp *httpResponse) (*[]Peer, error) {
	return &hResp.Peers, nil
}

func udpExtractPeers(uResp *udpResponse) (*[]Peer, error) {
	const peerSize = 6 // 4 bytes IP, 2 bytes Port
	numPeers := len(uResp.Peers) / peerSize
	fmt.Printf("Number of peers: %d\n", numPeers)

	if len(uResp.Peers)%peerSize != 0 {
		err := fmt.Errorf("malformed peers received from tracker")
		return nil, err
	}

	peers := make([]Peer, 0, numPeers)
	for i := 0; i < len(uResp.Peers); i += peerSize {
		ipBytes := uResp.Peers[i : i+4]
		port := binary.BigEndian.Uint16(uResp.Peers[i+4 : i+6])

		ipStr := fmt.Sprintf("%d.%d.%d.%d", ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3])

		peers = append(peers, Peer{
			IP:   ipStr,
			Port: port,
		})
	}

	return &peers, nil
}
