//go:build !windows

package main

func runPlatformServiceAction(action string) (string, error) {
	_ = action
	return "Windows Service is only available on Windows", nil
}
