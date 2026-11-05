//go:build !windows

package quota

import "errors"

// dpapiUnprotect: Cursor seals its sign-in with DPAPI, which only Windows has.
func dpapiUnprotect([]byte) ([]byte, error) {
	return nil, errors.New("DPAPI is Windows-only")
}
