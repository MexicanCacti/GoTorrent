package bencode

import (
	"bytes"
	"crypto/sha1"
	"io"
	"log"

	"github.com/jackpal/bencode-go"
)

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

type BencodeType struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

func Bytes(b BencodeType) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := bencode.Marshal(buf, b.Info)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ParseTorrent(reader io.Reader, path string) (TorrentType, error) {
	bencodeObject := BencodeType{}
	err := bencode.Unmarshal(reader, &bencodeObject)
	if err != nil {
		log.Printf("Error parsing torrent file: %v\n", err)
		return TorrentType{}, err
	}

	return convertToTorrent(bencodeObject, path)
}

func convertToTorrent(bencode BencodeType, path string) (TorrentType, error) {

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
	torrent.NumPieces = pieceCount

	return torrent, nil
}
