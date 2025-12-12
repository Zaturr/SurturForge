package ram

import (
	"fmt"
	"time"
)

// DisplayRAMBenchmarkSummary muestra un resumen completo de los resultados del benchmark de RAM
// con las métricas solicitadas: tamaño de bloque, iteraciones, duración, timestamp, frecuencia,
// canales de memoria, RAM usada durante el benchmark y cambio en uso
func DisplayRAMBenchmarkSummary(result *RAMBenchmarkResult) {
	if result == nil {
		fmt.Println("No hay resultados para mostrar")
		return
	}

	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("=== RESUMEN DEL BENCHMARK DE RAM ===")
	fmt.Println(repeat("=", 60))

	// Información básica del benchmark
	fmt.Println("\n--- INFORMACIÓN DEL BENCHMARK ---")
	fmt.Printf("  Tamaño del bloque: %s\n", formatBytes(result.BlockSize))
	fmt.Printf("  Iteraciones totales: %d\n", result.TotalIterations)
	fmt.Printf("  Duración total: %v\n", result.TotalDuration)
	fmt.Printf("  Timestamp: %s\n", result.Timestamp.Format(time.RFC3339))

	// Información de hardware
	fmt.Println("\n--- INFORMACIÓN DE HARDWARE ---")
	if result.FrequencyMHz > 0 {
		fmt.Printf("  Frecuencia: %d MHz\n", result.FrequencyMHz)
	} else {
		fmt.Printf("  Frecuencia: No disponible\n")
	}
	if result.MemoryChannels > 0 {
		fmt.Printf("  Canales de Memoria: %d\n", result.MemoryChannels)
	} else {
		fmt.Printf("  Canales de Memoria: No disponible\n")
	}

	// Métricas de RAM durante el benchmark
	fmt.Println("\n--- USO DE RAM DURANTE EL BENCHMARK ---")
	if result.RAMUsedBefore > 0 {
		fmt.Printf("  RAM usada antes: %s (%.1f%%)\n", formatBytes(result.RAMUsedBefore), result.RAMUsageBefore)
	}
	if result.RAMUsedAfter > 0 {
		fmt.Printf("  RAM usada después: %s (%.1f%%)\n", formatBytes(result.RAMUsedAfter), result.RAMUsageAfter)
	}
	if result.RAMUsedBefore > 0 || result.RAMUsedAfter > 0 {
		fmt.Printf("  Cambio en uso: %.1f%%\n", result.RAMUsageChange)
		if result.RAMUsageChange > 0 {
			fmt.Printf("    (Incremento de %.1f%% durante el benchmark)\n", result.RAMUsageChange)
		} else if result.RAMUsageChange < 0 {
			fmt.Printf("    (Reducción de %.1f%% durante el benchmark)\n", -result.RAMUsageChange)
		} else {
			fmt.Printf("    (Sin cambio significativo)\n")
		}
	} else {
		fmt.Printf("  Información de RAM: No disponible\n")
	}

	// Resumen de rendimiento
	fmt.Println("\n--- RENDIMIENTO ---")
	if result.SequentialWriteGBs > 0 {
		fmt.Printf("  Velocidad de Escritura: %.2f GB/s (%.2f MB/s)\n",
			result.SequentialWriteGBs, result.SequentialWriteMBs)
	}
	if result.SequentialReadGBs > 0 {
		fmt.Printf("  Velocidad de Lectura: %.2f GB/s (%.2f MB/s)\n",
			result.SequentialReadGBs, result.SequentialReadMBs)
	}
	if result.CopyGBs > 0 {
		fmt.Printf("  Velocidad de Copia: %.2f GB/s (%.2f MB/s)\n",
			result.CopyGBs, result.CopyMBs)
	}
	if result.LatencyNs > 0 {
		fmt.Printf("  Latencia promedio: %.2f ns\n", result.LatencyNs)
	}

	// Integridad de datos
	fmt.Println("\n--- INTEGRIDAD DE DATOS ---")
	if result.DataIntegrityOK {
		fmt.Printf("  Estado: TODAS las iteraciones pasaron la verificación\n")
	} else {
		fmt.Printf("  Estado: Se detectaron errores en algunas iteraciones\n")
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
