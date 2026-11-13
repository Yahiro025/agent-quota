//go:build windows

package quota

import (
	"bytes"
	b64enc "encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func dpapiProtect(t *testing.T, blob []byte) []byte {
	t.Helper()
	in := windows.DataBlob{Size: uint32(len(blob)), Data: &blob[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, 0x1, &out); err != nil {
		t.Fatal(err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

// TestDecodeCursorSafeStorage walks the whole path Cursor leaves on disk: Local State's
// DPAPI-wrapped key, then the v10 blob under cursorAuth/accessToken.
func TestDecodeCursorSafeStorage(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "Cursor", "User", "globalStorage", "state.vscdb")
	state := filepath.Join(dir, "Cursor", "Local State")

	key := bytes.Repeat([]byte{0x33}, 32)
	wrapped := append([]byte("DPAPI"), dpapiProtect(t, key)...)
	if err := os.MkdirAll(filepath.Dir(state), 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"os_crypt":{"encrypted_key":%q}}`, b64enc.StdEncoding.EncodeToString(wrapped))
	if err := os.WriteFile(state, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	token := "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1c2VyXzEifQ.c2ln"
	sealed := cursorSafeStoragePrefix + b64enc.StdEncoding.EncodeToString(sealCursorV10(t, key, []byte(token)))
	got, err := decodeCursorSafeStorage(sealed, db)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	eq(t, got, token, "sealed token")

	if _, err := decodeCursorSafeStorage(sealed, filepath.Join(dir, "elsewhere", "state.vscdb")); err == nil {
		t.Fatal("a missing Local State must not decode")
	}
}
