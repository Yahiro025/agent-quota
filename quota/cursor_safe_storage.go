package quota

import (
	"crypto/aes"
	"crypto/cipher"
	b64enc "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Cursor 3.27 seals cursorAuth/accessToken at rest once authTokenEncryptionAtRest is
// true: "cursor-safe-storage:v1:" then base64 of a Chromium v10 blob. The blob is
// AES-256-GCM over the JWT, keyed by the key Local State wraps in DPAPI. Reading it
// here keeps the quota working; the token stays read-only and in memory.
const cursorSafeStoragePrefix = "cursor-safe-storage:v1:"

var errCursorSafeStorage = errors.New("Cursor’s sealed sign-in couldn’t be opened")

// decodeCursorSafeStorage opens a value Cursor sealed, given where its database is: the
// key sits in Local State beside the Cursor folder.
func decodeCursorSafeStorage(text, db string) (string, error) {
	sealed, err := b64enc.StdEncoding.DecodeString(strings.TrimPrefix(text, cursorSafeStoragePrefix))
	if err != nil {
		return "", fmt.Errorf("%w: %v", errCursorSafeStorage, err)
	}
	state := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(db))), "Local State")
	key, err := cursorOSCryptKey(state)
	if err != nil {
		return "", err
	}
	token, err := openCursorV10(sealed, key)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errCursorSafeStorage, err)
	}
	return token, nil
}

// cursorOSCryptKey is Chromium's AES key from Local State: base64 of "DPAPI" then the
// blob only this Windows user can unseal.
func cursorOSCryptKey(localState string) ([]byte, error) {
	raw, err := os.ReadFile(localState)
	if err != nil {
		return nil, fmt.Errorf("the key file couldn’t be read: %v", err)
	}
	var state struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("the key file couldn’t be read: %v", err)
	}
	wrapped, err := b64enc.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil || !strings.HasPrefix(string(wrapped), "DPAPI") {
		return nil, errors.New("the key file holds no DPAPI key")
	}
	key, err := dpapiUnprotect(wrapped[len("DPAPI"):])
	if err != nil {
		return nil, fmt.Errorf("the key couldn’t be unsealed: %v", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("the key is %d bytes, not 32", len(key))
	}
	return key, nil
}

// openCursorV10 is Chromium's v10 envelope: "v10", a 12-byte nonce, then the AES-256-GCM
// ciphertext with its tag at the end.
func openCursorV10(sealed, key []byte) (string, error) {
	if len(sealed) < 3+12+16 || string(sealed[:3]) != "v10" {
		return "", errors.New("it is not a v10 blob")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	token, err := gcm.Open(nil, sealed[3:15], sealed[15:], nil)
	if err != nil {
		return "", err
	}
	return string(token), nil
}
