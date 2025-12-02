//go:build windows
// +build windows

package cpu

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/sensors"
)

func getCurrentClockSpeedWindows() (float64, error) {
	psCmd := `Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty CurrentClockSpeed`
	cmd := exec.Command("powershell", "-Command", psCmd)
	output, err := cmd.Output()
	if err == nil {
		value := strings.TrimSpace(string(output))
		if value != "" {
			freq, err := strconv.ParseFloat(value, 64)
			if err == nil && freq > 0 {
				return freq, nil
			}
		}
	}

	cmd = exec.Command("wmic", "cpu", "get", "CurrentClockSpeed", "/format:value")
	output, err = cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CurrentClockSpeed=") {
			value := strings.TrimPrefix(line, "CurrentClockSpeed=")
			value = strings.TrimSpace(value)
			if value != "" {
				freq, err := strconv.ParseFloat(value, 64)
				if err == nil && freq > 0 {
					return freq, nil
				}
			}
		}
	}

	return 0, nil
}

func getCurrentTemperatureWindows() (float64, error) {
	psCmd := `Get-CimInstance Win32_TemperatureProbe -ErrorAction SilentlyContinue | Where-Object { $_.Name -like '*CPU*' -or $_.Name -like '*Processor*' } | Select-Object -First 1 -ExpandProperty CurrentReading`
	cmd := exec.Command("powershell", "-Command", psCmd)
	output, err := cmd.Output()
	if err == nil {
		value := strings.TrimSpace(string(output))
		if value != "" {
			temp, err := strconv.ParseFloat(value, 64)
			if err == nil && temp > 0 && temp < 200 {
				if temp > 100 {
					return temp / 10.0, nil
				}
				return temp, nil
			}
		}
	}

	psCmd = `Get-CimInstance MSAcpi_ThermalZoneTemperature -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty CurrentTemperature`
	cmd = exec.Command("powershell", "-Command", psCmd)
	output, err = cmd.Output()
	if err == nil {
		value := strings.TrimSpace(string(output))
		if value != "" {
			temp, err := strconv.ParseFloat(value, 64)
			if err == nil && temp > 0 {
				celsius := (temp / 10.0) - 273.15
				if celsius > 0 && celsius < 150 {
					return celsius, nil
				}
			}
		}
	}

	return 0, nil
}

func GetCPUMetrics() (CPUMetrics, error) {
	metrics := CPUMetrics{
		Duration: time.Now(),
	}

	usagePercentages, err := cpu.Percent(time.Second, false)
	if err == nil && len(usagePercentages) > 0 {
		metrics.UsagePercent = usagePercentages[0]
	}

	currentFreq, err := getCurrentClockSpeedWindows()
	if err == nil && currentFreq > 0 {
		metrics.ClockSpeed = currentFreq
	} else {
		cpuInfos, err := cpu.Info()
		if err == nil && len(cpuInfos) > 0 {
			if cpuInfos[0].Mhz > 0 {
				metrics.ClockSpeed = cpuInfos[0].Mhz
			}
		}
	}

	physicalCores, err := cpu.Counts(false)
	if err == nil {
		metrics.Cores = physicalCores
	}

	logicalCores, err := cpu.Counts(true)
	if err == nil {
		metrics.Threads = logicalCores
	}

	temp, err := getCurrentTemperatureWindows()
	if err == nil && temp > 0 {
		metrics.Temperature = temp
	} else {
		temperatures, err := sensors.SensorsTemperatures()
		if err == nil {
			for _, temp := range temperatures {
				if temp.Temperature > 0 && temp.Temperature < 150 {
					if temp.SensorKey == "cpu" ||
						temp.SensorKey == "coretemp" ||
						temp.SensorKey == "cpu_thermal" ||
						temp.SensorKey == "Package id 0" {
						metrics.Temperature = temp.Temperature
						break
					} else if metrics.Temperature == 0 {
						metrics.Temperature = temp.Temperature
					}
				}
			}
		}
	}

	return metrics, nil
}
