package crypto

import (
	"bytes"
	"testing"
)

func TestAESGCMRoundTripAndRandomNonce(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	cipher, err := NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM() error = %v", err)
	}

	plaintext := []byte("tf_live_secret")
	first, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	second, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() second error = %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("Encrypt() reused a nonce")
	}

	got, err := cipher.Decrypt(first)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestAESGCMRejectsInvalidKeyAndTamperedCiphertext(t *testing.T) {
	if _, err := NewAESGCM([]byte("too short")); err == nil {
		t.Fatal("NewAESGCM() accepted an invalid key")
	}

	cipher, err := NewAESGCM(bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatalf("NewAESGCM() error = %v", err)
	}
	payload, err := cipher.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	payload[len(payload)-1] ^= 1
	if _, err := cipher.Decrypt(payload); err == nil {
		t.Fatal("Decrypt() accepted tampered ciphertext")
	}
}
