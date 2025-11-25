package cpu

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/sensors"
)

type CPUMetrics struct {
	UsagePercent float64 // porcentaje de uso
	ClockSpeed   float64 // MHz
	Temperature  float64 // Celsius (si está disponible)
	Cores        int     // número de cores físicos
	Threads      int     // número de threads lógicos
	Duration     time.Time
}

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
		// Usar la primera CPU para obtener la frecuencia
		if cpuInfos[0].Mhz > 0 {
			metrics.ClockSpeed = cpuInfos[0].Mhz
		}
	}

	// Obtener número de cores físicos
	physicalCores, err := cpu.Counts(false)
	if err == nil {
		metrics.Cores = physicalCores
	}

	// Obtener número de threads lógicos
	logicalCores, err := cpu.Counts(true)
	if err == nil {
		metrics.Threads = logicalCores
	}

	// Intentar obtener temperatura (puede no estar disponible en todos los sistemas)
	temperatures, err := sensors.SensorsTemperatures()
	if err == nil {
		// Buscar la temperatura de la CPU
		// Generalmente viene etiquetada como "cpu" o "coretemp" o similar
		for _, temp := range temperatures {
			// Algunos sistemas reportan la temperatura con diferentes nombres
			// Intentar encontrar la temperatura de CPU
			if temp.Temperature > 0 && temp.Temperature < 150 { // Rango razonable para CPU en Celsius
				// Preferir temperaturas etiquetadas como CPU
				if temp.SensorKey == "cpu" ||
					temp.SensorKey == "coretemp" ||
					temp.SensorKey == "cpu_thermal" ||
					temp.SensorKey == "Package id 0" {
					metrics.Temperature = temp.Temperature
					break
				} else if metrics.Temperature == 0 {
					// Si no encontramos una específica de CPU, usar la primera válida como fallback
					metrics.Temperature = temp.Temperature
				}
			}
		}
	}

	return metrics, nil
}
