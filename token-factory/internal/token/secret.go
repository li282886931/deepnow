package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var ErrRandomnessUnavailable = errors.New("secure randomness unavailable")

type SecretCipher interface {
	Encrypt([]byte) ([]byte, error)
}

func GenerateSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("%w: %v", ErrRandomnessUnavailable, err)
	}
	return "tf_live_" + hex.EncodeToString(raw[:]), nil
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
