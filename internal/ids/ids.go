package ids

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a cryptographically random, URL-safe 128-bit identifier.
func New() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(value[:])
}
