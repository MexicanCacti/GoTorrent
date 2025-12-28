package bencode

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sqweek/dialog"
)

const bytesPerChunk = 20

type TorrentFile struct {
	Path   string
	Length int64
	Offset int64
}
type TorrentType struct {
	Path        string
	Announce    string
	Name        string
	Length      int64
	PieceLength int64
	InfoHash    [bytesPerChunk]byte
	PieceHashes [][bytesPerChunk]byte
	PeerID      [20]byte
	NumPieces   int

	Files []TorrentFile
}

func (t TorrentType) CalcPieceSize(index int) int {
	if index < 0 || index >= t.NumPieces {
		return 0
	}

	if index == t.NumPieces-1 {
		remaining := int(t.Length - int64(index)*t.PieceLength)
		return remaining
	}

	return int(t.PieceLength)
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

func PickDownloadPath() (string, error) {
	path, err := dialog.Directory().Title("Select download location").Browse()
	if err != nil {
		return "", err
	}
	return path, nil
}

func OpenFiles(t *TorrentType, savePath string) ([]*os.File, error) {
	openFiles := make([]*os.File, len(t.Files))
	for i, f := range t.Files {
		fullPath := filepath.Join(savePath, f.Path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755) // perm 0755: allow dir creation, 0644: file, 0777: all
		if err != nil {
			log.Fatal(err)
		}

		file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}

		err = file.Truncate(f.Length)
		if err != nil {
			log.Fatal(err)
		}

		openFiles[i] = file
	}

	return openFiles, nil
}
