package quota

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

// sealCursorV10 builds what Chromium's safeStorage hands Cursor, so the reader's open can
// be tested without Cursor or DPAPI.
func sealCursorV10(t *testing.T, key, token []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{0x2a}, gcm.NonceSize())
	return gcm.Seal(append([]byte("v10"), nonce...), nonce, token, nil)
}

func TestOpenCursorV10(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	sealed := sealCursorV10(t, key, []byte("header.payload.signature"))
	got, err := openCursorV10(sealed, key)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	eq(t, got, "header.payload.signature", "roundtrip")

	if _, err := openCursorV10(sealed, bytes.Repeat([]byte{0x22}, 32)); err == nil {
		t.Fatal("a wrong key must not open the blob")
	}
	if _, err := openCursorV10([]byte("v11short"), key); err == nil {
		t.Fatal("a short, non-v10 blob must not open")
	}
}
