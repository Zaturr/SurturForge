//go:build windows
// +build windows

package ram

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func executeCommandWithTimeout(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}

	return output, err
}

func getRAMFrequencyWindows() uint64 {
	psCmd := `Get-CimInstance Win32_PhysicalMemory | Select-Object -First 1 -ExpandProperty Speed`
	output, err := executeCommandWithTimeout("powershell", "-Command", psCmd)
	if err == nil {
		value := strings.TrimSpace(string(output))
		if value != "" {
			freq, err := strconv.ParseUint(value, 10, 64)
			if err == nil && freq > 0 {
				return freq
			}
		}
	}

	output, err = executeCommandWithTimeout("wmic", "memorychip", "get", "Speed", "/format:value")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Speed=") {
			value := strings.TrimPrefix(line, "Speed=")
			value = strings.TrimSpace(value)
			if value != "" {
				freq, err := strconv.ParseUint(value, 10, 64)
				if err == nil && freq > 0 {
					return freq
				}
			}
		}
	}

	return 0
}

func getMemoryChannelsWindows() int {
	psCmd := `Get-CimInstance Win32_PhysicalMemory | Measure-Object | Select-Object -ExpandProperty Count`
	output, err := executeCommandWithTimeout("powershell", "-Command", psCmd)
	if err == nil {
		countStr := strings.TrimSpace(string(output))
		if countStr != "" {
			count, err := strconv.Atoi(countStr)
			if err == nil && count > 0 {
				if count >= 4 {
					return 4
				} else if count >= 2 {
					return 2
				} else {
					return 1
				}
			}
		}
	}

	output, err = executeCommandWithTimeout("wmic", "memorychip", "get", "DeviceLocator", "/format:value")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "DeviceLocator=") {
			count++
		}
	}

	if count > 0 {
		if count >= 4 {
			return 4
		} else if count >= 2 {
			return 2
		} else {
			return 1
		}
	}

	return 0
}
