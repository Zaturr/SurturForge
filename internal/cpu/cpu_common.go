package cpu

import (
	"fmt"
	"runtime"
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

// CalculateAdvancedMetrics calcula métricas avanzadas de CPU
func CalculateAdvancedMetrics(report *BenchmarkReport) map[string]interface{} {
	metrics := make(map[string]interface{})

	// Obtener número de cores
	var numCores int
	if report.BaselineResult != nil {
		numCores = report.BaselineResult.Hardware.CPUCoresLogical
		if numCores == 0 {
			numCores = report.BaselineResult.Hardware.CPUCoresPhysical
		}
	}

	// Si aún no tenemos cores, usar runtime como fallback
	if numCores == 0 {
		numCores = runtime.NumCPU()
	}

	// Parallel Efficiency - Usar promedio de SHA-256 y AES-256 para mayor precisión
	singleSHA := FindBenchmark(report.Benchmarks, "BenchmarkSHA256SingleCore")
	multiSHA := FindBenchmark(report.Benchmarks, "BenchmarkSHA256MultiCore")
	singleAES := FindBenchmark(report.Benchmarks, "BenchmarkAES256SingleCore")
	multiAES := FindBenchmark(report.Benchmarks, "BenchmarkAES256MultiCore")

	var shaSpeedup, aesSpeedup float64
	var speedupCount int

	// Calcular speedup para SHA-256
	if singleSHA != nil && multiSHA != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleSHA.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiSHA.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			shaSpeedup = float64(singleNs) / float64(multiNs)
			speedupCount++
		}
	}

	// Calcular speedup para AES-256
	if singleAES != nil && multiAES != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleAES.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiAES.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			aesSpeedup = float64(singleNs) / float64(multiNs)
			speedupCount++
		}
	}

	// Calcular promedio de speedups y Parallel Efficiency
	if speedupCount > 0 && numCores > 0 {
		var avgSpeedup float64
		if speedupCount == 2 {
			avgSpeedup = (shaSpeedup + aesSpeedup) / 2.0
		} else if shaSpeedup > 0 {
			avgSpeedup = shaSpeedup
		} else {
			avgSpeedup = aesSpeedup
		}

		parallelEfficiency := (avgSpeedup / float64(numCores)) * 100.0
		metrics["ParallelEfficiency"] = parallelEfficiency
		metrics["SpeedupFactor"] = avgSpeedup
	}

	// IPC (Instructions Per Cycle) - Estimación basada en rendimiento
	// IPC real requiere contadores de rendimiento, aquí hacemos una estimación
	// Para operaciones criptográficas (SHA-256, AES-256), el IPC típico es 0.5-2.0
	if report.SingleCoreScore > 0 {
		// Si no hay ClockSpeedAvg, usar un valor estimado basado en el CPU
		clockSpeed := 1000.0 // Valor por defecto (1 GHz) para estimación
		if report.Stats != nil && report.Stats.ClockSpeedAvg > 0 {
			clockSpeed = report.Stats.ClockSpeedAvg
		}

		// Base IPC: Score normalizado por frecuencia
		// Factor de corrección más conservador para operaciones criptográficas
		baseIPC := report.SingleCoreScore / clockSpeed
		correctionFactor := 0.6 // Ajustado para operaciones criptográficas intensivas
		estimatedIPC := baseIPC * correctionFactor

		// Ajustar si está fuera del rango razonable pero mantener el valor
		if estimatedIPC < 0.1 {
			estimatedIPC = 0.1 // Mínimo razonable
		} else if estimatedIPC > 5.0 {
			estimatedIPC = 5.0 // Máximo razonable
		}

		// Siempre mostrar el IPC estimado
		metrics["IPC"] = estimatedIPC
		if report.Stats == nil || report.Stats.ClockSpeedAvg <= 0 {
			metrics["IPCEstimated"] = true // Marcar como estimado
		}
	}

	// Thermal Throttling Detection
	throttlingEvents := 0
	if report.Stats != nil {
		// Verificar si tenemos datos de frecuencia válidos
		// ClockSpeedMin se inicializa en 100000.0, así que si es menor que eso, fue actualizado
		if report.Stats.ClockSpeedMin < 100000.0 && report.Stats.ClockSpeedMax > 0 {
			throttleRatio := report.Stats.ClockSpeedMin / report.Stats.ClockSpeedMax
			throttlePercent := (1.0 - throttleRatio) * 100.0

			if throttleRatio < 0.85 { // Si la frecuencia mínima es < 85% de la máxima, posible throttling
				throttlingEvents = 1
				if throttleRatio < 0.70 { // Throttling severo
					throttlingEvents = 2
				}
			}

			metrics["ThrottleRatio"] = throttleRatio
			metrics["ThrottlePercent"] = throttlePercent
		}
		// Si no hay datos de frecuencia min/max, asumir que no hubo throttling (0 eventos)
		metrics["ThermalThrottlingEvents"] = throttlingEvents
	} else {
		// Si no hay stats, no hubo throttling
		metrics["ThermalThrottlingEvents"] = 0
	}

	// Performance per Watt
	if report.MultiCoreScore > 0 {
		var power float64
		var isEstimated bool

		if report.Stats != nil && report.Stats.EnergyConsumption > 0 {
			power = report.Stats.EnergyConsumption
		} else {
			// Estimar consumo energético
			basePower := 10.0
			usageFactor := 0.5 // Valor por defecto si no hay datos
			freqFactor := 1.0  // Valor por defecto (normalizado a 3 GHz)

			if report.Stats != nil {
				if report.Stats.Average > 0 {
					usageFactor = report.Stats.Average / 100.0
				}
				if report.Stats.ClockSpeedAvg > 0 {
					freqFactor = report.Stats.ClockSpeedAvg / 3000.0
				}
			}

			thermalFactor := 1.0
			if report.Stats != nil && report.Stats.TemperatureAvg > 0 {
				thermalFactor = 1.0 + (report.Stats.TemperatureAvg-40.0)/100.0
			}

			power = basePower + (usageFactor * freqFactor * thermalFactor * 50.0)
			isEstimated = true
		}

		if power > 0 {
			perfPerWatt := report.MultiCoreScore / power
			metrics["PerformancePerWatt"] = perfPerWatt
			if isEstimated {
				metrics["PowerEstimated"] = true
			}
		}
	}

	// Baseline Comparison
	if report.BaselineResult != nil {
		baselineCPUUsage := 100.0 - report.BaselineResult.Baseline.CPUIdlePercent
		currentCPUUsage := 0.0

		if report.Stats != nil && report.Stats.Average > 0 {
			currentCPUUsage = report.Stats.Average
		} else if report.Stats != nil {
			// Si no hay Average, intentar usar un valor estimado
			currentCPUUsage = 50.0 // Valor por defecto conservador
		}

		if currentCPUUsage > 0 {
			metrics["BaselineCPUUsage"] = baselineCPUUsage
			metrics["CurrentCPUUsage"] = currentCPUUsage
			metrics["CPUUsageDelta"] = currentCPUUsage - baselineCPUUsage

			// Comparación porcentual (con protección contra división por cero)
			if baselineCPUUsage > 0.1 {
				comparison := (currentCPUUsage / baselineCPUUsage) * 100.0
				metrics["CPUUsageComparison"] = comparison
			} else if baselineCPUUsage > 0 {
				// Si baseline es muy bajo pero > 0, calcular de forma diferente
				// Mostrar cuántas veces mayor es el uso actual
				comparison := currentCPUUsage * 10.0 // Escalar para mostrar diferencia
				metrics["CPUUsageComparison"] = comparison
			} else {
				// Baseline es 0, mostrar 100% (uso normal durante benchmark)
				metrics["CPUUsageComparison"] = 100.0
			}
		}
	}

	return metrics
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

	// Calcular métricas avanzadas
	advancedMetrics := CalculateAdvancedMetrics(report)

	// Mostrar tabla de métricas esenciales
	printSection("CPU - Top 10 Métricas Esenciales")
	fmt.Println()
	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("%-35s %-30s %-15s %-10s\n", "Métrica", "Fórmula/Descripción", "Valor", "Estado")
	fmt.Println(strings.Repeat("-", 90))

	// Single-Core Score
	fmt.Printf("%-35s %-30s %-15.2f %-10s\n",
		"Single-Core Score",
		"Puntuación sintética mononúcleo",
		report.SingleCoreScore,
		"")

	// Multi-Core Score
	fmt.Printf("%-35s %-30s %-15.2f %-10s\n",
		"Multi-Core Score",
		"Puntuación sintética multinúcleo",
		report.MultiCoreScore,
		"")

	// Parallel Efficiency
	if val, ok := advancedMetrics["ParallelEfficiency"].(float64); ok {
		status := ""
		if val >= 70 {
			status = ""
		} else if val >= 50 {
			status = ""
		} else {
			status = ""
		}
		fmt.Printf("%-35s %-30s %-15.2f%% %-10s\n",
			"Parallel Efficiency",
			"Speedup / Number of Cores × 100",
			val,
			status)
	} else {
		fmt.Printf("%-35s %-30s %-15s %-10s\n",
			"Parallel Efficiency",
			"Speedup / Number of Cores × 100",
			"N/A",
			"")
	}

	// IPC
	if val, ok := advancedMetrics["IPC"].(float64); ok {
		status := ""
		if val < 1.5 {
			status = ""
		}
		estimated := ""
		if _, isEst := advancedMetrics["IPCEstimated"].(bool); isEst {
			estimated = " (estimado)"
		}
		fmt.Printf("%-35s %-30s %-15.2f%s %-10s\n",
			"IPC (Instructions/Cycle)",
			"Instrucciones por ciclo (estimado)",
			val,
			estimated,
			status)
	} else {
		// Calcular estimación básica si no está disponible
		if report.SingleCoreScore > 0 {
			clockSpeed := 1000.0 // Valor por defecto
			if report.Stats != nil && report.Stats.ClockSpeedAvg > 0 {
				clockSpeed = report.Stats.ClockSpeedAvg
			}
			baseIPC := report.SingleCoreScore / clockSpeed
			estimatedIPC := baseIPC * 0.6
			if estimatedIPC < 0.1 {
				estimatedIPC = 0.1
			} else if estimatedIPC > 5.0 {
				estimatedIPC = 5.0
			}
			fmt.Printf("%-35s %-30s %-15.2f (estimado) %-10s\n",
				"IPC (Instructions/Cycle)",
				"Instrucciones por ciclo (estimado)",
				estimatedIPC,
				"")
		} else {
			fmt.Printf("%-35s %-30s %-15s %-10s\n",
				"IPC (Instructions/Cycle)",
				"Instrucciones por ciclo",
				"N/A",
				"")
		}
	}

	// CPU Utilization
	if report.Stats != nil && report.Stats.Samples > 0 {
		status := ""
		if report.Stats.Average < 30 {
			status = ""
		} else if report.Stats.Average > 95 {
			status = ""
		}
		fmt.Printf("%-35s %-30s %-15.1f%% %-10s\n",
			"CPU Utilization",
			"Uso promedio durante benchmark",
			report.Stats.Average,
			status)
	}

	// Thermal Throttling
	if val, ok := advancedMetrics["ThermalThrottlingEvents"].(int); ok {
		status := ""
		if val > 0 {
			status = ""
		}
		fmt.Printf("%-35s %-30s %-15d eventos %-10s\n",
			"Thermal Throttling",
			"Eventos de throttling",
			val,
			status)
	} else {
		// Si no hay datos, asumir 0 eventos (no throttling detectado)
		fmt.Printf("%-35s %-30s %-15d eventos %-10s\n",
			"Thermal Throttling",
			"Eventos de throttling",
			0,
			"")
	}

	// Performance per Watt
	if val, ok := advancedMetrics["PerformancePerWatt"].(float64); ok {
		status := ""
		if val < 10 {
			status = ""
		}
		estimated := ""
		if _, isEst := advancedMetrics["PowerEstimated"].(bool); isEst {
			estimated = " (estimado)"
		}
		fmt.Printf("%-35s %-30s %-15.2f%s %-10s\n",
			"Performance per Watt",
			"Score / Power (W)",
			val,
			estimated,
			status)
	} else {
		// Calcular estimación si no está disponible
		if report.MultiCoreScore > 0 {
			// Estimación conservadora
			estimatedPower := 30.0 // Valor típico para CPU de escritorio
			perfPerWatt := report.MultiCoreScore / estimatedPower
			fmt.Printf("%-35s %-30s %-15.2f (estimado) %-10s\n",
				"Performance per Watt",
				"Score / Power (W)",
				perfPerWatt,
				"")
		} else {
			fmt.Printf("%-35s %-30s %-15s %-10s\n",
				"Performance per Watt",
				"Score / Power (W)",
				"N/A",
				"")
		}
	}

	// Cache Hit Rate (L3) - No disponible sin contadores de rendimiento
	fmt.Printf("%-35s %-30s %-15s %-10s\n",
		"Cache Hit Rate (L3)",
		"(Cache Hits / Total Accesses) × 100",
		"N/A",
		"")

	// Temperature
	if report.Stats != nil && report.Stats.TemperatureAvg > 0 {
		status := ""
		if report.Stats.TemperatureAvg > 80 {
			status = ""
		}
		if report.Stats.TemperatureAvg > 90 {
			status = ""
		}
		fmt.Printf("%-35s %-30s %-15.1f°C %-10s\n",
			"Temperature",
			"Temperatura promedio",
			report.Stats.TemperatureAvg,
			status)
	}

	// Baseline Comparison
	if comparison, ok := advancedMetrics["CPUUsageComparison"].(float64); ok {
		status := ""
		// Normalizar comparación si es > 1000 (caso especial cuando baseline es muy bajo)
		displayValue := comparison
		if comparison > 1000 {
			// Si fue escalado, mostrar como porcentaje relativo
			baselineUsage, _ := advancedMetrics["BaselineCPUUsage"].(float64)
			currentUsage, _ := advancedMetrics["CurrentCPUUsage"].(float64)
			if baselineUsage > 0 {
				displayValue = (currentUsage / baselineUsage) * 100.0
			} else {
				displayValue = 100.0 // Valor por defecto
			}
		}

		if displayValue > 0 {
			if displayValue < 95 {
				status = ""
			}
			if displayValue < 85 {
				status = ""
			}
			fmt.Printf("%-35s %-30s %-15.1f%% %-10s\n",
				"Baseline Comparison",
				"(Current / Baseline) × 100",
				displayValue,
				status)
		} else {
			fmt.Printf("%-35s %-30s %-15.1f%% %-10s\n",
				"Baseline Comparison",
				"(Current / Baseline) × 100",
				100.0,
				"")
		}
	} else if baselineUsage, ok := advancedMetrics["BaselineCPUUsage"].(float64); ok {
		// Fallback: calcular comparación directamente
		currentUsage, _ := advancedMetrics["CurrentCPUUsage"].(float64)
		if baselineUsage > 0.1 && currentUsage > 0 {
			comparison := (currentUsage / baselineUsage) * 100.0
			status := ""
			if comparison < 95 {
				status = ""
			}
			if comparison < 85 {
				status = ""
			}
			fmt.Printf("%-35s %-30s %-15.1f%% %-10s\n",
				"Baseline Comparison",
				"(Current / Baseline) × 100",
				comparison,
				status)
		} else {
			fmt.Printf("%-35s %-30s %-15.1f%% %-10s\n",
				"Baseline Comparison",
				"(Current / Baseline) × 100",
				100.0,
				"")
		}
	} else {
		// Si no hay baseline, mostrar 100% (sin comparación)
		fmt.Printf("%-35s %-30s %-15s %-10s\n",
			"Baseline Comparison",
			"(Current / Baseline) × 100",
			"N/A (sin baseline)",
			"")
	}

	fmt.Println(strings.Repeat("=", 90))
	fmt.Println()

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
		fmt.Printf("Muestras: %d (mediciones de uso de CPU tomadas cada 100ms durante el benchmark)\n", report.Stats.Samples)

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

	//printSection("Explicación")
	//fmt.Println("• Tiempo/Op: Tiempo promedio por operación (menor = mejor)")
	//fmt.Println("• Iteraciones: Número de veces que se ejecutó el benchmark")
	//fmt.Println("• Mejora Multi-Core: Factor de mejora usando múltiples cores")
	//fmt.Println("• CPU: Uso del procesador durante la ejecución del benchmark")
	//fmt.Println("• Puntuaciones Sintéticas: Basadas en rendimiento de benchmarks (mayor = mejor)")
	//fmt.Println("• Consumo Energético: Estimado basado en uso, frecuencia y temperatura")
}
