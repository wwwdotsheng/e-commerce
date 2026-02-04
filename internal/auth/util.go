package auth

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
