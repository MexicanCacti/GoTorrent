package peer_discovery

import (
	"GoTorrent/bencode"
	"GoTorrent/safeio"
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

	jackpal "github.com/jackpal/bencode-go"
)

type Torrent = bencode.TorrentType
type SafeWriter = safeio.SafeWriter
type SafeReader = safeio.SafeReader

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

func GetPeers(t *Torrent, peerID [20]byte, port uint16) (*[]Peer, error) {
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

func buildHTTP(t *bencode.TorrentType, peerID [20]byte, port uint16) (*[]Peer, error) {
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
		log.Println(fmt.Sprintf("http query tracker: %s", err))
		return nil, err
	}

	httpResponse := httpResponse{}

	err = jackpal.Unmarshal(bytes.NewReader(respBody), &httpResponse)
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
func buildUDP(t *Torrent, peerID [20]byte, port uint16) (*[]Peer, error) {
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
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			return
		}
	}(conn)
	transactionID := rand.Uint32()

	// Connect input
	buf := new(bytes.Buffer)
	protocolID := uint64(0x41727101980) //Note magic constant for udp tracker
	action := uint32(0)

	safeWriter := safeio.NewSafeWriter(buf)
	safeWriter.WriteBigEndian(protocolID)
	safeWriter.WriteBigEndian(action)
	safeWriter.WriteBigEndian(transactionID)

	if safeWriter.GetError() != nil {
		return nil, safeWriter.GetError()
	}

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

	safeReader := safeio.NewSafeReader(respBuf)
	safeReader.ReadBigEndian(&respAction)
	safeReader.ReadBigEndian(&respTransactionID)
	safeReader.ReadBigEndian(&respConnectionID)

	if safeReader.GetError() != nil {
		return nil, safeReader.GetError()
	}

	if respAction != 0 {
		return nil, fmt.Errorf("invalid response action %d", respAction)
	}
	if respTransactionID != transactionID {
		return nil, fmt.Errorf("invalid transaction ID %d", respTransactionID)
	}

	transactionID = rand.Uint32()
	// Announce input
	buf = new(bytes.Buffer)

	safeWriter = safeio.NewSafeWriter(buf)
	safeWriter.WriteBigEndian(respConnectionID)
	safeWriter.WriteBigEndian(uint32(1)) // action
	safeWriter.WriteBigEndian(transactionID)

	buf.Write(t.InfoHash[:])
	buf.Write(peerID[:])

	safeWriter.WriteBigEndian(uint64(0))        // downloaded
	safeWriter.WriteBigEndian(uint64(t.Length)) // left
	safeWriter.WriteBigEndian(uint64(0))        // uploaded
	safeWriter.WriteBigEndian(uint32(0))        // event
	safeWriter.WriteBigEndian(uint32(0))        // IP address
	safeWriter.WriteBigEndian(rand.Uint32())    // key
	safeWriter.WriteBigEndian(int32(-1))        // num_want
	safeWriter.WriteBigEndian(port)

	if safeWriter.GetError() != nil {
		return nil, safeWriter.GetError()
	}

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
