//go:build windows

package quota

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapiUnprotect unwraps a DPAPI blob, which only this Windows user can read.
func dpapiUnprotect(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, errors.New("empty DPAPI blob")
	}
	in := windows.DataBlob{Size: uint32(len(blob)), Data: &blob[0]}
	var out windows.DataBlob
	// CRYPTPROTECT_UI_FORBIDDEN: never prompt behind the widget.
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0x1, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
