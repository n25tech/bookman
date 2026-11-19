package library

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
)

// HashSize is how much of the start of a file goes into its content hash.
const HashSize = 1 << 20

// ContentHash identifies a book by sha256(first 1 MiB || file size), so a moved
// or renamed file keeps its identity (and with it progress and notes) without
// reading large files completely.
func ContentHash(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	if _, err := io.CopyN(h, f, HashSize); err != nil && err != io.EOF {
		return "", 0, err
	}
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(st.Size()))
	h.Write(size[:])
	return hex.EncodeToString(h.Sum(nil)), st.Size(), nil
}
