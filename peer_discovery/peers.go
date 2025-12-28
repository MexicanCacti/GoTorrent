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

const protocolID uint64 = 0x41727101980 //Note magic constant for udp tracker
const udpMaxRetries = 8
const udpConnectAction = uint32(0)
const udpAnnounceAction = uint32(1)
const udpWait = 15 * time.Second

type udpResponse struct {
	Interval uint64 `bencode:"interval"`
	Peers    []byte `bencode:"peers"`
}

type httpResponse struct {
	Complete   uint64      `bencode:"complete"`
	Incomplete uint64      `bencode:"incomplete"`
	Interval   uint64      `bencode:"interval"`
	Peers      interface{} `bencode:"peers"`
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
	params := url.Values{}
	params.Add("info_hash", string(t.InfoHash[:]))
	params.Add("peer_id", string(peerID[:]))
	params.Add("port", strconv.Itoa(int(port)))
	params.Add("uploaded", strconv.Itoa(0))
	params.Add("downloaded", strconv.Itoa(0))
	params.Add("left", strconv.Itoa(0))
	params.Add("compact", strconv.Itoa(1))

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

	raddr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	connID, err := udpConnect(conn, raddr)
	if err != nil {
		return nil, err
	}

	return udpAnnounce(conn, raddr, connID, t, peerID, port)
	/*
		transactionID := rand.Uint32()

		// Connect input
		buf := new(bytes.Buffer)
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
		conn.SetReadDeadline(time.Time{})
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
	*/
}

func udpConnect(conn *net.UDPConn, raddr *net.UDPAddr) (uint64, error) {
	timeout := udpWait
	for attempt := 0; attempt < udpMaxRetries; attempt++ {
		transactionID := rand.Uint32()

		// Connect input
		buf := new(bytes.Buffer)
		safeWriter := safeio.NewSafeWriter(buf)
		safeWriter.WriteBigEndian(protocolID)
		safeWriter.WriteBigEndian(udpConnectAction)
		safeWriter.WriteBigEndian(transactionID)

		if safeWriter.GetError() != nil {
			return 0, safeWriter.GetError()
		}

		_, err := conn.WriteToUDP(buf.Bytes(), raddr)
		if err != nil {
			return 0, err
		}
		/*
			binary.BigEndian.PutUint64(buf[0:8], protocolID)
			binary.BigEndian.PutUint64(buf[8:12], udpConnectAction)
			binary.BigEndian.PutUint32(buf[12:16], transactionID)
		*/
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		resp := make([]byte, 16)
		n, _, err := conn.ReadFromUDP(resp)
		if err != nil {
			timeout *= 2
			continue
		}
		if n < 16 {
			timeout *= 2
			continue
		}

		// Connect output
		respBuf := bytes.NewBuffer(resp)
		var respAction uint32
		var respTransactionID uint32
		var respConnectionID uint64
		safeReader := safeio.NewSafeReader(respBuf)
		safeReader.ReadBigEndian(&respAction)
		safeReader.ReadBigEndian(&respTransactionID)
		safeReader.ReadBigEndian(&respConnectionID)

		if safeReader.GetError() != nil {
			return 0, safeReader.GetError()
		}
		if respAction != udpConnectAction || respTransactionID != transactionID {
			timeout *= 2
			continue
		}

		return respConnectionID, nil
	}

	return 0, fmt.Errorf("failed to connect to %s", raddr.String())
}

func udpAnnounce(conn *net.UDPConn, raddr *net.UDPAddr, respConnectionID uint64, t *Torrent, peerID [20]byte, port uint16) (*[]Peer, error) {
	timeout := udpWait
	for attempt := 0; attempt < udpMaxRetries; attempt++ {
		transactionID := rand.Uint32()
		// Announce input
		buf := new(bytes.Buffer)

		safeWriter := safeio.NewSafeWriter(buf)
		safeWriter.WriteBigEndian(respConnectionID)
		safeWriter.WriteBigEndian(uint32(1)) // action
		safeWriter.WriteBigEndian(transactionID)

		buf.Write(t.InfoHash[:])
		buf.Write(peerID[:])

		safeWriter.WriteBigEndian(uint64(0))        // downloaded
		safeWriter.WriteBigEndian(uint64(t.Length)) // left
		safeWriter.WriteBigEndian(uint64(0))        // uploaded
		safeWriter.WriteBigEndian(uint32(2))        // event
		safeWriter.WriteBigEndian(uint32(0))        // IP address
		safeWriter.WriteBigEndian(rand.Uint32())    // key
		safeWriter.WriteBigEndian(int32(-1))        // num_want
		safeWriter.WriteBigEndian(port)

		if safeWriter.GetError() != nil {
			return nil, safeWriter.GetError()
		}
		_, err := conn.WriteToUDP(buf.Bytes(), raddr)
		if err != nil {
			return nil, err
		}

		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		/*
			buf := make([]byte, 98)
			binary.BigEndian.PutUint64(buf[0:8], respConnectionID)
			binary.BigEndian.PutUint32(buf[8:12], udpAnnounceAction)
			binary.BigEndian.PutUint32(buf[12:16], transactionID)

			copy(buf[16:36], t.InfoHash[:])
			copy(buf[36:56], peerID[:])

			binary.BigEndian.PutUint64(buf[56:64], 0) // downloaded
			binary.BigEndian.PutUint64(buf[64:72], uint64(t.Length))
			binary.BigEndian.PutUint64(buf[72:80], 0)             // uploaded
			binary.BigEndian.PutUint32(buf[80:84], 2)             // event=started
			binary.BigEndian.PutUint32(buf[84:88], 0)             // IP
			binary.BigEndian.PutUint32(buf[88:92], rand.Uint32()) // key
			binary.BigEndian.PutUint32(buf[92:96], uint32(0xffffffff))
			binary.BigEndian.PutUint16(buf[96:98], port)
		*/

		// Announce output
		announceBuf := make([]byte, 1500) // Hopefully good enough, 1472 theoretical max message size
		n, _, err := conn.ReadFromUDP(announceBuf)
		if err != nil {
			timeout *= 2
			continue
		}
		if n < 20 {
			timeout *= 2
			continue
		}

		announceResp := announceBuf[:n]

		fmt.Printf("Size of resp: %v", n)
		var announceAction = binary.BigEndian.Uint32(announceResp[0:4])
		var announceTransactionID = binary.BigEndian.Uint32(announceResp[4:8])

		if announceAction != udpAnnounceAction || announceTransactionID != transactionID {
			timeout *= 2
			continue
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
	return nil, fmt.Errorf("failed announce to %s", raddr.String())
}

func httpExtractPeers(hResp *httpResponse) (*[]Peer, error) {
	switch peers := hResp.Peers.(type) {
	case string:
		return parseCompactPeers([]byte(peers))

	case []interface{}:
		return parseDictPeers(peers)
	default:
		return nil, fmt.Errorf("invalid peer format")
	}
}

func parseCompactPeers(data []byte) (*[]Peer, error) {
	const peerSize = 6
	if len(data)%peerSize != 0 {
		return nil, fmt.Errorf("invalid compact peer format")
	}

	peers := make([]Peer, 0, len(data)/peerSize)

	for i := 0; i < len(data); i += peerSize {
		ip := net.IP(data[i : i+4]).String()
		port := binary.BigEndian.Uint16(data[i+4 : i+6])
		peers = append(peers, Peer{
			IP:   ip,
			Port: port,
		})
	}
	return &peers, nil
}

func parseDictPeers(list []interface{}) (*[]Peer, error) {
	peers := make([]Peer, 0, len(list))

	for _, p := range list {
		m := p.(map[string]interface{})

		peers = append(peers, Peer{
			IP:   m["ip"].(string),
			Port: uint16(m["port"].(int64)),
		})
	}

	return &peers, nil
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
