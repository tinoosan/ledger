package v1

import (
	"crypto/sha256"
	"encoding/hex"
)

type storedBatch struct {
	BodyHash string
	Status   int
	Payload  []byte
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
