package securestore

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	t.Setenv("AISH_ENCRYPTION_KEY", "test-passphrase")
	in := []byte("private data")
	enc, err := Encrypt(in)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, in) {
		t.Fatal("expected encrypted data")
	}
	out, err := Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, in) {
		t.Fatalf("got %q", out)
	}
}
