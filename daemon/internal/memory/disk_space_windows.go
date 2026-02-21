//go:build windows

package memory

import "errors"

func availableDiskBytes(path string) (uint64, error) {
	return 0, errors.New("disk free-space probe unsupported on windows build")
}
