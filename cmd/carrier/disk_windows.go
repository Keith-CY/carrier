//go:build windows

package main

import "errors"

func statfsFreeBytes(_ string) (uint64, error) {
	return 0, errors.New("disk check unsupported on Windows")
}
