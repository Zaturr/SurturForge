package cpu

import (
	"fmt"
	"strings"
	"time"

	"v2/internal/baseline"

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

// CPUStats contiene estadísticas de CPU durante un benchmark
type CPUStats struct {
	Min, Max, Average float64
	Samples           int
	// Métricas adicionales
	TemperatureMin    float64
	TemperatureMax    float64
	TemperatureAvg    float64
	ClockSpeedMin     float64
	ClockSpeedMax     float64
	ClockSpeedAvg     float64
	EnergyConsumption float64 // Watts estimados
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
}

// BenchmarkResult representa el resultado de un benchmark
type BenchmarkResult struct {
	Name       string
	Iterations string
	TimePerOp  string
	Allocs     string
	Bytes      string
}

// BenchmarkReport contiene todos los datos del reporte de benchmarks
type BenchmarkReport struct {
	Benchmarks      map[string]*BenchmarkResult
	SingleCoreScore float64
	MultiCoreScore  float64
	Stats           *CPUStats
	BaselineResult  *baseline.BaselineResult
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

// MonitorCPU monitorea el CPU durante la ejecución de un benchmark
func MonitorCPU(done chan struct{}) *CPUStats {
	stats := &CPUStats{
		Min:            100.0,
		TemperatureMin: 1000.0,
		ClockSpeedMin:  100000.0,
		StartTime:      time.Now(),
	}
	var totalUsage, totalTemp, totalClock float64
	var tempSamples, clockSamples int
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			stats.EndTime = time.Now()
			stats.Duration = stats.EndTime.Sub(stats.StartTime)
			if stats.Samples > 0 {
				stats.Average = totalUsage / float64(stats.Samples)
			}
			if tempSamples > 0 {
				stats.TemperatureAvg = totalTemp / float64(tempSamples)
			}
			if clockSamples > 0 {
				stats.ClockSpeedAvg = totalClock / float64(clockSamples)
			}
			// Calcular consumo energético estimado
			// Estimación basada en: uso de CPU, frecuencia y temperatura
			if stats.Samples > 0 && stats.ClockSpeedAvg > 0 {
				// Fórmula simplificada: consumo base + (uso% * frecuencia normalizada * factor térmico)
				basePower := 10.0 // Consumo base estimado en watts
				usageFactor := stats.Average / 100.0
				freqFactor := stats.ClockSpeedAvg / 3000.0 // Normalizar a 3GHz
				thermalFactor := 1.0
				if stats.TemperatureAvg > 0 {
					thermalFactor = 1.0 + (stats.TemperatureAvg-40.0)/100.0 // Factor térmico
				}
				stats.EnergyConsumption = basePower + (usageFactor * freqFactor * thermalFactor * 50.0)
			}
			return stats
		case <-ticker.C:
			if m, err := GetCPUMetrics(); err == nil {
				usage := m.UsagePercent
				stats.Samples++
				totalUsage += usage
				if usage < stats.Min {
					stats.Min = usage
				}
				if usage > stats.Max {
					stats.Max = usage
				}

				// Temperatura
				if m.Temperature > 0 {
					tempSamples++
					totalTemp += m.Temperature
					if m.Temperature < stats.TemperatureMin {
						stats.TemperatureMin = m.Temperature
					}
					if m.Temperature > stats.TemperatureMax {
						stats.TemperatureMax = m.Temperature
					}
				}

				// Frecuencia de reloj
				if m.ClockSpeed > 0 {
					clockSamples++
					totalClock += m.ClockSpeed
					if m.ClockSpeed < stats.ClockSpeedMin {
						stats.ClockSpeedMin = m.ClockSpeed
					}
					if m.ClockSpeed > stats.ClockSpeedMax {
						stats.ClockSpeedMax = m.ClockSpeed
					}
				}
			}
		}
	}
}

// ParseBenchmarkOutput parsea la salida de go test -bench
func ParseBenchmarkOutput(output string) map[string]*BenchmarkResult {
	lines := strings.Split(output, "\n")
	benchmarks := make(map[string]*BenchmarkResult)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Benchmark") && !strings.HasPrefix(line, "goos:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				result := &BenchmarkResult{
					Name:       parts[0],
					Iterations: parts[1],
					TimePerOp:  parts[2],
				}
				if len(parts) >= 5 {
					result.Allocs = parts[3]
					result.Bytes = parts[4]
				}
				benchmarks[result.Name] = result
			}
		}
	}
	return benchmarks
}

// FindBenchmark busca un benchmark por nombre
func FindBenchmark(benchmarks map[string]*BenchmarkResult, name string) *BenchmarkResult {
	if result, ok := benchmarks[name]; ok {
		return result
	}
	for k, v := range benchmarks {
		if strings.HasPrefix(k, name+"-") {
			return v
		}
	}
	return nil
}

// CalculateSyntheticScores calcula las puntuaciones sintéticas
func CalculateSyntheticScores(benchmarks map[string]*BenchmarkResult) (singleCoreScore, multiCoreScore float64) {
	// Puntuación Single-Core
	singleSHA := FindBenchmark(benchmarks, "BenchmarkSHA256SingleCore")
	singleAES := FindBenchmark(benchmarks, "BenchmarkAES256SingleCore")
	if singleSHA != nil && singleAES != nil {
		var shaNs, aesNs int64
		fmt.Sscanf(singleSHA.TimePerOp, "%d", &shaNs)
		fmt.Sscanf(singleAES.TimePerOp, "%d", &aesNs)
		if shaNs > 0 && aesNs > 0 {
			// Puntuación basada en rendimiento inverso (mayor tiempo = menor puntuación)
			// Normalizar y combinar ambos benchmarks
			shaScore := 1000000.0 / float64(shaNs) * 1000 // Escalar
			aesScore := 1000000.0 / float64(aesNs) * 1000
			singleCoreScore = (shaScore + aesScore) / 2.0
		}
	}

	// Puntuación Multi-Core
	multiSHA := FindBenchmark(benchmarks, "BenchmarkSHA256MultiCore")
	multiAES := FindBenchmark(benchmarks, "BenchmarkAES256MultiCore")
	if multiSHA != nil && multiAES != nil {
		var shaNs, aesNs int64
		fmt.Sscanf(multiSHA.TimePerOp, "%d", &shaNs)
		fmt.Sscanf(multiAES.TimePerOp, "%d", &aesNs)
		if shaNs > 0 && aesNs > 0 {
			shaScore := 1000000.0 / float64(shaNs) * 1000
			aesScore := 1000000.0 / float64(aesNs) * 1000
			multiCoreScore = (shaScore + aesScore) / 2.0
		}
	}

	return singleCoreScore, multiCoreScore
}

// FormatTime formatea tiempo de nanosegundos a formato legible
func FormatTime(ns string) string {
	var nanoseconds int64
	if _, err := fmt.Sscanf(ns, "%d", &nanoseconds); err != nil {
		return ns + " ns/op"
	}

	seconds := float64(nanoseconds) / 1_000_000_000.0
	if seconds >= 1.0 {
		return fmt.Sprintf("%.4f s/op", seconds)
	} else if seconds >= 0.001 {
		milliseconds := seconds * 1000
		return fmt.Sprintf("%.2f ms/op", milliseconds)
	} else {
		microseconds := seconds * 1_000_000
		return fmt.Sprintf("%.2f µs/op", microseconds)
	}
}

// GenerateBenchmarkReport genera un reporte completo de benchmarks
func GenerateBenchmarkReport(output string, stats *CPUStats) (*BenchmarkReport, error) {
	benchmarks := ParseBenchmarkOutput(output)

	// Obtener baseline para comparación
	var baselineResult *baseline.BaselineResult
	if baselineData, err := baseline.RunBaseline(); err == nil {
		baselineResult = &baselineData
	}

	// Calcular puntuaciones sintéticas
	singleCoreScore, multiCoreScore := CalculateSyntheticScores(benchmarks)

	return &BenchmarkReport{
		Benchmarks:      benchmarks,
		SingleCoreScore: singleCoreScore,
		MultiCoreScore:  multiCoreScore,
		Stats:           stats,
		BaselineResult:  baselineResult,
	}, nil
}

// DisplayReport muestra el reporte de benchmarks en consola
func DisplayReport(report *BenchmarkReport, colorBold, colorReset, colorCyan, colorGreen, colorYellow, colorRed string, printHeader, printSection func(string)) {
	if report == nil {
		return
	}

	printHeader("RESULTADOS BENCHMARKS")

	// Mostrar benchmarks principales
	fmt.Printf("%s%-35s %12s %20s%s\n", colorBold, "Benchmark", "Iteraciones", "Tiempo/Op", colorReset)
	fmt.Println(strings.Repeat("-", 70))

	for name, result := range report.Benchmarks {
		fmt.Printf("%-35s %12s %20s\n", name, result.Iterations, FormatTime(result.TimePerOp))
	}

	// Calcular y mostrar Puntuaciones Sintéticas
	printSection("Puntuaciones Sintéticas (Theoretical Performance)")

	fmt.Printf("Puntuación Single-Core (Mononúcleo): %s%.2f%s", colorCyan, report.SingleCoreScore, colorReset)
	if report.BaselineResult != nil {
		fmt.Printf(" | Sin baseline: %.2f", report.SingleCoreScore)
	}
	fmt.Println()

	fmt.Printf("Puntuación Multi-Core (Multinúcleo): %s%.2f%s", colorCyan, report.MultiCoreScore, colorReset)
	if report.BaselineResult != nil {
		fmt.Printf(" | Sin baseline: %.2f", report.MultiCoreScore)
	}
	fmt.Println()

	// Comparación Single-Core vs Multi-Core
	singleSHA := FindBenchmark(report.Benchmarks, "BenchmarkSHA256SingleCore")
	multiSHA := FindBenchmark(report.Benchmarks, "BenchmarkSHA256MultiCore")
	if singleSHA != nil && multiSHA != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleSHA.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiSHA.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			improvement := float64(singleNs) / float64(multiNs)
			fmt.Printf("\n%sSHA-256:%s Single: %s | Multi: %s | %s%.2fx más rápido%s\n",
				colorCyan, colorReset,
				FormatTime(singleSHA.TimePerOp),
				FormatTime(multiSHA.TimePerOp),
				colorGreen, improvement, colorReset)
		}
	}

	singleAES := FindBenchmark(report.Benchmarks, "BenchmarkAES256SingleCore")
	multiAES := FindBenchmark(report.Benchmarks, "BenchmarkAES256MultiCore")
	if singleAES != nil && multiAES != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleAES.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiAES.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			improvement := float64(singleNs) / float64(multiNs)
			fmt.Printf("%sAES-256:%s Single: %s | Multi: %s | %s%.2fx más rápido%s\n",
				colorCyan, colorReset,
				FormatTime(singleAES.TimePerOp),
				FormatTime(multiAES.TimePerOp),
				colorGreen, improvement, colorReset)
		}
	}

	// Datos de Monitoreo y Eficiencia
	printSection("Datos de Monitoreo y Eficiencia")

	// Consumo Energético
	if report.Stats != nil && report.Stats.EnergyConsumption > 0 {
		fmt.Printf("Consumo Energético: %s%.2f W%s", colorYellow, report.Stats.EnergyConsumption, colorReset)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %.2f W", report.Stats.EnergyConsumption)
		}
		fmt.Println()
	}

	// Temperaturas
	if report.Stats != nil && report.Stats.TemperatureAvg > 0 {
		fmt.Printf("Temperaturas: Min: %s%.1f°C%s | Max: %s%.1f°C%s | Promedio: %s%.1f°C%s",
			colorGreen, report.Stats.TemperatureMin, colorReset,
			colorRed, report.Stats.TemperatureMax, colorReset,
			colorYellow, report.Stats.TemperatureAvg, colorReset)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %.1f°C", report.Stats.TemperatureAvg)
		}
		fmt.Println()
	}

	// Frecuencia de Reloj
	if report.Stats != nil && report.Stats.ClockSpeedAvg > 0 {
		fmt.Printf("Frecuencia de Reloj: Min: %s%.2f MHz%s | Max: %s%.2f MHz%s | Promedio: %s%.2f MHz%s",
			colorGreen, report.Stats.ClockSpeedMin, colorReset,
			colorRed, report.Stats.ClockSpeedMax, colorReset,
			colorYellow, report.Stats.ClockSpeedAvg, colorReset)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %.2f MHz", report.Stats.ClockSpeedAvg)
		}
		fmt.Println()
	}

	// Uso de CPU (%)
	if report.Stats != nil && report.Stats.Samples > 0 {
		fmt.Printf("Uso de CPU (%%): Mínimo: %s%.1f%%%s | Máximo: %s%.1f%%%s | Promedio: %s%.1f%%%s",
			colorGreen, report.Stats.Min, colorReset,
			colorRed, report.Stats.Max, colorReset,
			colorYellow, report.Stats.Average, colorReset)
		if report.BaselineResult != nil {
			// Comparar con baseline
			baselineCPUUsage := 100.0 - report.BaselineResult.Baseline.CPUIdlePercent
			diff := report.Stats.Average - baselineCPUUsage
			fmt.Printf(" | Baseline: %.1f%%, Diferencia: %+.1f%%", baselineCPUUsage, diff)
			fmt.Printf(" | Sin baseline: %.1f%%", report.Stats.Average)
		}
		fmt.Println()
		fmt.Printf("Muestras: %d\n", report.Stats.Samples)

		// Interpretación
		if report.Stats.Average < 30 {
			fmt.Printf("%s CPU con bajo uso durante el benchmark%s\n", colorGreen, colorReset)
		} else if report.Stats.Average < 70 {
			fmt.Printf("%s CPU con uso moderado durante el benchmark%s\n", colorYellow, colorReset)
		} else {
			fmt.Printf("%s CPU con alto uso durante el benchmark%s\n", colorRed, colorReset)
		}
	}

	// Métricas de Tiempo
	printSection("Métricas de Tiempo")
	if report.Stats != nil && report.Stats.Duration > 0 {
		fmt.Printf("Duración Total: %s%v%s", colorCyan, report.Stats.Duration, colorReset)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %v", report.Stats.Duration)
		}
		fmt.Println()
		fmt.Printf("Inicio: %s | Fin: %s\n",
			report.Stats.StartTime.Format("15:04:05"),
			report.Stats.EndTime.Format("15:04:05"))
	}

	// Explicación
	printSection("Explicación")
	fmt.Println("• Tiempo/Op: Tiempo promedio por operación (menor = mejor)")
	fmt.Println("• Iteraciones: Número de veces que se ejecutó el benchmark")
	fmt.Println("• Mejora Multi-Core: Factor de mejora usando múltiples cores")
	fmt.Println("• CPU: Uso del procesador durante la ejecución del benchmark")
	fmt.Println("• Puntuaciones Sintéticas: Basadas en rendimiento de benchmarks (mayor = mejor)")
	fmt.Println("• Consumo Energético: Estimado basado en uso, frecuencia y temperatura")
}
