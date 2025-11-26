package cpu

import (
	"fmt"
	"strings"
	"time"

	"v2/internal/baseline"
)

// MonitorCPU monitorea el CPU durante la ejecución de un benchmark
func MonitorCPU(done chan struct{}) *CPUStats {
	stats := &CPUStats{
		Min:            100.0,
		TemperatureMin: 1000.0,
		ClockSpeedMin:  100000.0,
		TemperatureMax: 0.0,
		ClockSpeedMax:  0.0,
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
			if stats.Samples > 0 && stats.ClockSpeedAvg > 0 {
				basePower := 10.0
				usageFactor := stats.Average / 100.0
				freqFactor := stats.ClockSpeedAvg / 3000.0
				thermalFactor := 1.0
				if stats.TemperatureAvg > 0 {
					thermalFactor = 1.0 + (stats.TemperatureAvg-40.0)/100.0
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

func CalculateSyntheticScores(benchmarks map[string]*BenchmarkResult) (singleCoreScore, multiCoreScore float64) {
	singleSHA := FindBenchmark(benchmarks, "BenchmarkSHA256SingleCore")
	singleAES := FindBenchmark(benchmarks, "BenchmarkAES256SingleCore")
	if singleSHA != nil && singleAES != nil {
		var shaNs, aesNs int64
		fmt.Sscanf(singleSHA.TimePerOp, "%d", &shaNs)
		fmt.Sscanf(singleAES.TimePerOp, "%d", &aesNs)
		if shaNs > 0 && aesNs > 0 {
			shaScore := 1000000.0 / float64(shaNs) * 1000
			aesScore := 1000000.0 / float64(aesNs) * 1000
			singleCoreScore = (shaScore + aesScore) / 2.0
		}
	}

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

func GenerateBenchmarkReport(output string, stats *CPUStats) (*BenchmarkReport, error) {
	benchmarks := ParseBenchmarkOutput(output)

	var baselineResult *baseline.BaselineResult
	if baselineData, err := baseline.RunBaseline(); err == nil {
		baselineResult = &baselineData
	}

	singleCoreScore, multiCoreScore := CalculateSyntheticScores(benchmarks)

	return &BenchmarkReport{
		Benchmarks:      benchmarks,
		SingleCoreScore: singleCoreScore,
		MultiCoreScore:  multiCoreScore,
		Stats:           stats,
		BaselineResult:  baselineResult,
	}, nil
}

func DisplayReport(report *BenchmarkReport, printHeader, printSection func(string)) {
	if report == nil {
		return
	}

	printHeader("RESULTADOS BENCHMARKS")

	fmt.Printf("%-35s %12s %20s\n", "Benchmark", "Iteraciones", "Tiempo/Op")
	fmt.Println(strings.Repeat("-", 70))

	for name, result := range report.Benchmarks {
		fmt.Printf("%-35s %12s %20s\n", name, result.Iterations, FormatTime(result.TimePerOp))
	}

	printSection("Puntuaciones Sintéticas (Theoretical Performance)")

	fmt.Printf("Puntuación Single-Core (Mononúcleo): %.2f", report.SingleCoreScore)
	if report.BaselineResult != nil {
		fmt.Printf(" | Sin baseline: %.2f", report.SingleCoreScore)
	}
	fmt.Println()

	fmt.Printf("Puntuación Multi-Core (Multinúcleo): %.2f", report.MultiCoreScore)
	if report.BaselineResult != nil {
		fmt.Printf(" | Sin baseline: %.2f", report.MultiCoreScore)
	}
	fmt.Println()

	singleSHA := FindBenchmark(report.Benchmarks, "BenchmarkSHA256SingleCore")
	multiSHA := FindBenchmark(report.Benchmarks, "BenchmarkSHA256MultiCore")
	if singleSHA != nil && multiSHA != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleSHA.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiSHA.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			improvement := float64(singleNs) / float64(multiNs)
			fmt.Printf("\nSHA-256: Single: %s | Multi: %s | %.2fx más rápido\n",
				FormatTime(singleSHA.TimePerOp),
				FormatTime(multiSHA.TimePerOp),
				improvement)
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
			fmt.Printf("AES-256: Single: %s | Multi: %s | %.2fx más rápido\n",
				FormatTime(singleAES.TimePerOp),
				FormatTime(multiAES.TimePerOp),
				improvement)
		}
	}

	printSection("Datos de Monitoreo y Eficiencia")

	if report.Stats != nil && report.Stats.EnergyConsumption > 0 {
		fmt.Printf("Consumo Energético: %.2f W", report.Stats.EnergyConsumption)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %.2f W", report.Stats.EnergyConsumption)
		}
		fmt.Println()
	}

	if report.Stats != nil && report.Stats.TemperatureAvg > 0 {
		fmt.Printf("Temperaturas: Min: %.1f°C | Max: %.1f°C | Promedio: %.1f°C",
			report.Stats.TemperatureMin,
			report.Stats.TemperatureMax,
			report.Stats.TemperatureAvg)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %.1f°C", report.Stats.TemperatureAvg)
		}
		fmt.Println()
	}

	if report.Stats != nil && report.Stats.ClockSpeedAvg > 0 {
		fmt.Printf("Frecuencia de Reloj: Min: %.2f MHz | Max: %.2f MHz | Promedio: %.2f MHz",
			report.Stats.ClockSpeedMin,
			report.Stats.ClockSpeedMax,
			report.Stats.ClockSpeedAvg)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %.2f MHz", report.Stats.ClockSpeedAvg)
		}
		fmt.Println()
	}

	if report.Stats != nil && report.Stats.Samples > 0 {
		fmt.Printf("Uso de CPU (%%): Mínimo: %.1f%% | Máximo: %.1f%% | Promedio: %.1f%%",
			report.Stats.Min,
			report.Stats.Max,
			report.Stats.Average)
		if report.BaselineResult != nil {
			baselineCPUUsage := 100.0 - report.BaselineResult.Baseline.CPUIdlePercent
			diff := report.Stats.Average - baselineCPUUsage
			fmt.Printf(" | Baseline: %.1f%%, Diferencia: %+.1f%%", baselineCPUUsage, diff)
			fmt.Printf(" | Sin baseline: %.1f%%", report.Stats.Average)
		}
		fmt.Println()
		fmt.Printf("Muestras: %d\n", report.Stats.Samples)

		if report.Stats.Average < 30 {
			fmt.Printf("CPU con bajo uso durante el benchmark\n")
		} else if report.Stats.Average < 70 {
			fmt.Printf("CPU con uso moderado durante el benchmark\n")
		} else {
			fmt.Printf("CPU con alto uso durante el benchmark\n")
		}
	}

	printSection("Métricas de Tiempo")
	if report.Stats != nil && report.Stats.Duration > 0 {
		fmt.Printf("Duración Total: %v", report.Stats.Duration)
		if report.BaselineResult != nil {
			fmt.Printf(" | Sin baseline: %v", report.Stats.Duration)
		}
		fmt.Println()
		fmt.Printf("Inicio: %s | Fin: %s\n",
			report.Stats.StartTime.Format("15:04:05"),
			report.Stats.EndTime.Format("15:04:05"))
	}

	printSection("Explicación")
	fmt.Println("• Tiempo/Op: Tiempo promedio por operación (menor = mejor)")
	fmt.Println("• Iteraciones: Número de veces que se ejecutó el benchmark")
	fmt.Println("• Mejora Multi-Core: Factor de mejora usando múltiples cores")
	fmt.Println("• CPU: Uso del procesador durante la ejecución del benchmark")
	fmt.Println("• Puntuaciones Sintéticas: Basadas en rendimiento de benchmarks (mayor = mejor)")
	fmt.Println("• Consumo Energético: Estimado basado en uso, frecuencia y temperatura")
}
