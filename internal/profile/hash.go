package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func SHA256Bytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func SHA256File(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}
