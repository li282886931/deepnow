package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

var ErrInvalidKey = errors.New("AES-GCM key must be 16, 24, or 32 bytes")

type AESGCM struct {
	aead cipher.AEAD
}

func NewAESGCM(key []byte) (*AESGCM, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{aead: aead}, nil
}

func (c *AESGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *AESGCM) Decrypt(payload []byte) ([]byte, error) {
	if len(payload) < c.aead.NonceSize() {
		return nil, errors.New("AES-GCM ciphertext is too short")
	}
	nonce := payload[:c.aead.NonceSize()]
	return c.aead.Open(nil, nonce, payload[c.aead.NonceSize():], nil)
}
