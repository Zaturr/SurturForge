//go:build !windows
// +build !windows

package cpu

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/sensors"
)

func GetCPUMetrics() (CPUMetrics, error) {
	metrics := CPUMetrics{
		Duration: time.Now(),
	}

	// Obtener porcentaje de uso de CPU
	usagePercentages, err := cpu.Percent(time.Second, false)
	if err == nil && len(usagePercentages) > 0 {
		metrics.UsagePercent = usagePercentages[0]
	}

	// Obtener información de CPU para velocidad del reloj
	cpuInfos, err := cpu.Info()
	if err == nil && len(cpuInfos) > 0 {
		if cpuInfos[0].Mhz > 0 {
			metrics.ClockSpeed = cpuInfos[0].Mhz
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

	return metrics, nil
}
