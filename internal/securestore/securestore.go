package securestore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

const prefix = "AISHENC1:"

func key() []byte {
	pass := os.Getenv("AISH_ENCRYPTION_KEY")
	if pass == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(pass))
	return sum[:]
}

func Encrypt(data []byte) ([]byte, error) {
	k := key()
	if len(k) == 0 {
		return data, nil
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, data, nil)
	raw := append(nonce, sealed...)
	out := prefix + base64.StdEncoding.EncodeToString(raw)
	return []byte(out), nil
}

func Decrypt(data []byte) ([]byte, error) {
	if len(data) < len(prefix) || string(data[:len(prefix)]) != prefix {
		return data, nil
	}
	k := key()
	if len(k) == 0 {
		return nil, fmt.Errorf("encrypted AISH data requires AISH_ENCRYPTION_KEY")
	}
	raw, err := base64.StdEncoding.DecodeString(string(data[len(prefix):]))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted payload")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
}
