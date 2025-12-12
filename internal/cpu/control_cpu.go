package cpu

import (
	"fmt"
	"time"
)

// DisplayCPUBenchmarkSummary muestra un resumen completo de los resultados del benchmark de CPU
// con las métricas solicitadas: CPU usado, puntuaciones single/multi-core, tiempo de ejecución,
// y MIPS/GIPS/FLOPS
func DisplayCPUBenchmarkSummary(report *BenchmarkReport) {
	if report == nil {
		fmt.Println("No hay resultados para mostrar")
		return
	}

	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("=== RESUMEN DEL BENCHMARK DE CPU ===")
	fmt.Println(repeat("=", 60))

	// Uso de CPU durante el benchmark
	fmt.Println("\n--- USO DE CPU DURANTE EL BENCHMARK ---")
	if report.CPUUsageBefore >= 0 {
		fmt.Printf("  CPU usado antes: %.1f%%\n", report.CPUUsageBefore)
	}
	if report.CPUUsageAfter >= 0 {
		fmt.Printf("  CPU usado después: %.1f%%\n", report.CPUUsageAfter)
	}
	if report.CPUUsageBefore >= 0 || report.CPUUsageAfter >= 0 {
		fmt.Printf("  Cambio en uso: %.1f%%\n", report.CPUUsageChange)
		if report.CPUUsageChange > 0 {
			fmt.Printf("    (Incremento de %.1f%% durante el benchmark)\n", report.CPUUsageChange)
		} else if report.CPUUsageChange < 0 {
			fmt.Printf("    (Reducción de %.1f%% durante el benchmark)\n", -report.CPUUsageChange)
		} else {
			fmt.Printf("    (Sin cambio significativo)\n")
		}
	}
	if report.Stats != nil && report.Stats.Samples > 0 {
		fmt.Printf("  CPU promedio durante benchmark: %.1f%%\n", report.Stats.Average)
		fmt.Printf("  CPU mínimo: %.1f%% | CPU máximo: %.1f%%\n", report.Stats.Min, report.Stats.Max)
	}

	// Puntuaciones
	fmt.Println("\n--- PUNTUACIONES ---")
	if report.SingleCoreScore > 0 {
		fmt.Printf("  Puntuación de un Solo Núcleo (Single-Core Score): %.2f\n", report.SingleCoreScore)
	}
	if report.MultiCoreScore > 0 {
		fmt.Printf("  Puntuación Multinúcleo (Multi-Core Score): %.2f\n", report.MultiCoreScore)
	}
	if report.SingleCoreScore > 0 && report.MultiCoreScore > 0 {
		improvement := (report.MultiCoreScore / report.SingleCoreScore) * 100
		fmt.Printf("  Mejora Multi-Core: %.1f%%\n", improvement)
	}

	// Tiempo de Ejecución
	fmt.Println("\n--- TIEMPO DE EJECUCIÓN ---")
	if report.Stats != nil && report.Stats.Duration > 0 {
		fmt.Printf("  Duración total: %v\n", report.Stats.Duration)
		fmt.Printf("  Inicio: %s\n", report.Stats.StartTime.Format(time.RFC3339))
		fmt.Printf("  Fin: %s\n", report.Stats.EndTime.Format(time.RFC3339))
	} else {
		fmt.Printf("  Información de tiempo: No disponible\n")
	}

	// MIPS / GIPS / FLOPS
	fmt.Println("\n--- RENDIMIENTO (MIPS / GIPS / FLOPS) ---")
	if report.MIPS > 0 {
		if report.MIPS >= 1000 {
			fmt.Printf("  MIPS: %.2f (%.4f GIPS)\n", report.MIPS, report.GIPS)
		} else {
			fmt.Printf("  MIPS: %.2f\n", report.MIPS)
		}
	}
	if report.GIPS > 0 {
		fmt.Printf("  GIPS: %.4f\n", report.GIPS)
	}
	if report.FLOPS > 0 {
		fmt.Printf("  FLOPS: %.4f GFLOPS\n", report.FLOPS)
	}
	if report.MIPS == 0 && report.GIPS == 0 && report.FLOPS == 0 {
		fmt.Printf("  Información de rendimiento: No disponible\n")
	}

	// Información adicional de hardware
	if report.Stats != nil {
		fmt.Println("\n--- INFORMACIÓN ADICIONAL ---")
		if report.Stats.ClockSpeedAvg > 0 {
			fmt.Printf("  Frecuencia promedio: %.2f MHz\n", report.Stats.ClockSpeedAvg)
			if report.Stats.ClockSpeedMin > 0 && report.Stats.ClockSpeedMax > 0 {
				fmt.Printf("  Frecuencia: Min: %.2f MHz | Max: %.2f MHz\n",
					report.Stats.ClockSpeedMin, report.Stats.ClockSpeedMax)
			}
		}
		if report.Stats.TemperatureAvg > 0 {
			fmt.Printf("  Temperatura promedio: %.1f°C\n", report.Stats.TemperatureAvg)
			if report.Stats.TemperatureMin > 0 && report.Stats.TemperatureMax > 0 {
				fmt.Printf("  Temperatura: Min: %.1f°C | Max: %.1f°C\n",
					report.Stats.TemperatureMin, report.Stats.TemperatureMax)
			}
		}
		if report.Stats.EnergyConsumption > 0 {
			fmt.Printf("  Consumo de energía estimado: %.2f W\n", report.Stats.EnergyConsumption)
		}
	}

	fmt.Println("\n" + repeat("=", 60))
}

// Helper function para repetir strings
func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
