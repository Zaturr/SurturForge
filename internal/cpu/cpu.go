package cpu

import (
	"os/exec"
	"runtime"
)

func GetCPUInfo() (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("wmic", "cpu", "get", "name,numberofcores,numberOfLogicalProcessors,maxclockspeed", "/format:list")
	} else {
		cmd = exec.Command("lscpu")
	}
	cpuInfo, err := cmd.Output()
	if err != nil {
		return "", err
	}
	info := string(cpuInfo)
	if runtime.GOOS == "windows" {
		info = formatWindowsCPUInfo(info)
	}
	return info, nil
}

func formatWindowsCPUInfo(info string) string {
	return info
}
