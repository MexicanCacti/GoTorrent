package torrentstruct

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/jackpal/bencode-go"
	"github.com/sqweek/dialog"
)

const bytesPerChunk = 20

type bencodeFile struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
}
type bencodeInfo struct {
	Pieces      string        `bencode:"pieces"`
	PieceLength int64         `bencode:"piece length"`
	Length      int64         `bencode:"length,omitempty"`
	Name        string        `bencode:"name"`
	Files       []bencodeFile `bencode:"files,omitempty"`
}

type bencodeType struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

type TorrentType struct {
	Path        string
	Announce    string
	Name        string
	Length      int64
	PieceLength int64
	InfoHash    [bytesPerChunk]byte
	PieceHashes [][bytesPerChunk]byte
}

func Bytes(b bencodeType) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := bencode.Marshal(buf, b.Info)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (t TorrentType) String() string {
	var outputString strings.Builder

	outputString.WriteString(fmt.Sprintf("Torrent Path: %s\n", t.Path))

	outputString.WriteString("Torrent Info:\n")
	outputString.WriteString(fmt.Sprintf("\tName: %s\n", t.Name))
	outputString.WriteString(fmt.Sprintf("\tLength: %d\n", t.Length))
	outputString.WriteString(fmt.Sprintf("\tPieceLength: %d\n", t.PieceLength))
	outputString.WriteString(fmt.Sprintf("\tPieces: %d hashes\n", len(t.PieceHashes)))
	outputString.WriteString(fmt.Sprintf("\tInfoHash: %x\n", t.InfoHash))

	return outputString.String()
}

func PickTorrent() (string, error) {
	path, err := dialog.File().
		Filter("Torrent File", "torrent").
		Title("Select a .torrent file").
		Load()
	if err != nil {
		return "", err
	}
	return path, nil
}

func ParseTorrent(reader io.Reader, path string) (TorrentType, error) {
	bencodeObject := bencodeType{}
	err := bencode.Unmarshal(reader, &bencodeObject)
	if err != nil {
		log.Printf("Error parsing torrent file: %v\n", err)
		return TorrentType{}, err
	}

	return convertToTorrent(bencodeObject, path)
}

func convertToTorrent(bencode bencodeType, path string) (TorrentType, error) {

	torrent := TorrentType{}
	torrent.Path = path

	torrent.Announce = bencode.Announce
	torrent.Name = bencode.Info.Name
	torrent.PieceLength = bencode.Info.PieceLength

	if bencode.Info.Length > 0 {
		torrent.Length = bencode.Info.Length
	} else if len(bencode.Info.Files) > 0 {
		var totalLength int64
		for _, file := range bencode.Info.Files {
			totalLength += file.Length
		}
		torrent.Length = totalLength
	}

	pieceCount := len(bencode.Info.Pieces) / bytesPerChunk
	torrent.PieceHashes = make([][bytesPerChunk]byte, pieceCount)
	for i := 0; i < pieceCount; i++ {
		copy(torrent.PieceHashes[i][:], bencode.Info.Pieces[i*bytesPerChunk:(i+1)*bytesPerChunk])
	}

	infoBytes, err := Bytes(bencode)
	if err != nil {
		return torrent, nil
	}
	torrent.InfoHash = sha1.Sum(infoBytes)

	return torrent, nil
}
