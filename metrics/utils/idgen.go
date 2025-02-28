package utils

import (
	"crypto/rand"
	"encoding/hex"
)

func GenerateServerID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "fallback-id"
	}
	return hex.EncodeToString(bytes)
}
