//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runPlatformServiceAction(action string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	serviceName := "carrier"
	var cmd *exec.Cmd
	switch action {
	case "install":
		binPath, err := exec.LookPath("carrier.exe")
		if err != nil {
			binPath = "carrier.exe"
		}
		cmd = exec.Command("sc.exe", "create", serviceName, "binPath=", binPath, "start=", "auto")
	case "start":
		cmd = exec.Command("sc.exe", "start", serviceName)
	case "stop":
		cmd = exec.Command("sc.exe", "stop", serviceName)
	case "status":
		cmd = exec.Command("sc.exe", "query", serviceName)
	case "uninstall":
		cmd = exec.Command("sc.exe", "delete", serviceName)
	default:
		return "", fmt.Errorf("unsupported service action: %s", action)
	}
	raw, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(raw))
	if err != nil {
		if output == "" {
			return "", err
		}
		return "", fmt.Errorf("%w (%s)", err, output)
	}
	return output, nil
}
