//go:build windows

package memory

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32DLL            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExProc = kernel32DLL.NewProc("GetDiskFreeSpaceExW")
)

func availableDiskBytes(path string) (uint64, error) {
	if path == "" {
		path = "."
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return 0, fmt.Errorf("resolve disk space path: %w", err)
	}
	ptr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return 0, fmt.Errorf("encode path for GetDiskFreeSpaceExW: %w", err)
	}

	var freeBytes uint64
	r1, _, callErr := getDiskFreeSpaceExProc.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		0,
		0,
	)
	if r1 == 0 {
		if callErr != syscall.Errno(0) {
			return 0, fmt.Errorf("GetDiskFreeSpaceExW %s: %w", absPath, callErr)
		}
		return 0, fmt.Errorf("GetDiskFreeSpaceExW %s failed", absPath)
	}

	return freeBytes, nil
}
