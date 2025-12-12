package disk

import (
	"fmt"
	"time"
)

// DisplayDiskBenchmarkSummary muestra un resumen completo de los resultados del benchmark de disco
// con las métricas solicitadas: velocidad secuencial, velocidad aleatoria (4KB), latencia e IOPS
func DisplayDiskBenchmarkSummary(result *Result) {
	if result == nil {
		fmt.Println("No hay resultados para mostrar")
		return
	}

	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("=== RESUMEN DEL BENCHMARK DE DISCO ===")
	fmt.Println(repeat("=", 60))

	// 1. Velocidad Secuencial (Lectura/Escritura) en MB/s
	fmt.Println("\n--- VELOCIDAD SECUENCIAL ---")
	if result.WriteThroughputMBs > 0 {
		fmt.Printf("  Escritura: %.2f MB/s\n", result.WriteThroughputMBs)
	}
	if result.ReadThroughputMBs > 0 {
		fmt.Printf("  Lectura:   %.2f MB/s\n", result.ReadThroughputMBs)
	}
	if result.WriteThroughputMBs > 0 && result.ReadThroughputMBs > 0 {
		fmt.Printf("  Promedio:  %.2f MB/s\n", (result.WriteThroughputMBs+result.ReadThroughputMBs)/2.0)
	}

	// 2. Velocidad Aleatoria (4KB) - Lectura/Escritura en IOPS
	fmt.Println("\n--- VELOCIDAD ALEATORIA (4KB) ---")
	if result.RandomReadIOPS > 0 {
		fmt.Printf("  Lectura:  %.0f IOPS\n", result.RandomReadIOPS)
	}
	if result.RandomWriteIOPS > 0 {
		fmt.Printf("  Escritura: %.0f IOPS\n", result.RandomWriteIOPS)
	}
	if result.RandomReadIOPS > 0 && result.RandomWriteIOPS > 0 {
		totalIOPS := result.RandomReadIOPS + result.RandomWriteIOPS
		fmt.Printf("  Total:     %.0f IOPS\n", totalIOPS)
	}

	// 3. Latencia - Tiempo de Acceso (ms/ns)
	fmt.Println("\n--- LATENCIA (Tiempo de Acceso) ---")

	// Latencia secuencial
	if result.WriteLatency > 0 {
		fmt.Printf("  Escritura Secuencial: %s", formatLatency(result.WriteLatency))
	}
	if result.ReadLatency > 0 {
		fmt.Printf("  Lectura Secuencial:   %s", formatLatency(result.ReadLatency))
	}

	// Latencia aleatoria (4KB)
	if result.RandomReadLatency > 0 {
		fmt.Printf("  Lectura Aleatoria (4KB):  %s", formatLatency(result.RandomReadLatency))
	}
	if result.RandomWriteLatency > 0 {
		fmt.Printf("  Escritura Aleatoria (4KB): %s", formatLatency(result.RandomWriteLatency))
	}

	// 4. IOPS General
	fmt.Println("\n--- IOPS GENERAL ---")
	if result.IODuration > 0 && result.IOOperations > 0 {
		totalIOPS := float64(result.IOOperations) / result.IODuration.Seconds()
		fmt.Printf("  Total IOPS: %.0f\n", totalIOPS)
		fmt.Printf("  Operaciones: %d\n", result.IOOperations)
		fmt.Printf("  Duración:    %v\n", result.IODuration)
	}

	fmt.Println("\n" + repeat("=", 60))
}

// formatLatency formatea la latencia en ms o ns según su magnitud
func formatLatency(latency time.Duration) string {
	if latency >= time.Millisecond {
		return fmt.Sprintf("%.3f ms\n", float64(latency.Nanoseconds())/1e6)
	} else if latency >= time.Microsecond {
		return fmt.Sprintf("%.3f µs\n", float64(latency.Nanoseconds())/1e3)
	} else {
		return fmt.Sprintf("%.2f ns\n", float64(latency.Nanoseconds()))
	}
}

// Helper function para repetir strings (no disponible en Go estándar)
func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
