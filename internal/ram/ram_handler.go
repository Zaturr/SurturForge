package ram

import (
	"fmt"
	"math"
	"runtime"
	"time"
)

type RAMBenchmarkConfig struct {
	BlockSizeMB    int
	TargetDuration time.Duration
	MinIterations  int
}

type BenchmarkIterationResult struct {
	WriteGBs  float64
	ReadGBs   float64
	CopyGBs   float64
	LatencyNs float64
	WriteTime time.Duration
	ReadTime  time.Duration
	CopyTime  time.Duration
	DataValid bool
	Iteration int
}

type RAMBenchmarkResult struct {
	BlockSize          uint64
	TotalIterations    int
	TotalDuration      time.Duration
	SequentialWriteGBs float64
	SequentialWriteMBs float64
	SequentialReadGBs  float64
	SequentialReadMBs  float64
	CopyGBs            float64
	CopyMBs            float64
	LatencyNs          float64
	WriteMinGBs        float64
	WriteMaxGBs        float64
	WriteStdDev        float64
	ReadMinGBs         float64
	ReadMaxGBs         float64
	ReadStdDev         float64
	CopyMinGBs         float64
	CopyMaxGBs         float64
	CopyStdDev         float64
	LatencyMinNs       float64
	LatencyMaxNs       float64
	LatencyStdDev      float64
	FrequencyMHz       uint64
	MemoryChannels     int
	DataIntegrityOK    bool
	Iterations         []BenchmarkIterationResult
	Timestamp          time.Time
	// Métricas de RAM durante el benchmark
	RAMUsedBefore  uint64
	RAMUsedAfter   uint64
	RAMUsageBefore float64
	RAMUsageAfter  float64
	RAMUsageChange float64
}

func DefaultRAMBenchmarkConfig() RAMBenchmarkConfig {
	return RAMBenchmarkConfig{
		BlockSizeMB:    512,
		TargetDuration: 60 * time.Second,
		MinIterations:  3,
	}
}

func RunRAMBenchmark(config RAMBenchmarkConfig) (*RAMBenchmarkResult, error) {
	if config.TargetDuration == 0 {
		config.TargetDuration = 60 * time.Second
	}
	if config.MinIterations < 1 {
		config.MinIterations = 3
	}

	blockSize := uint64(config.BlockSizeMB) * 1024 * 1024

	metrics, err := GetRAMMetrics()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo métricas de RAM: %w", err)
	}

	requiredMemory := blockSize * 2
	if metrics.AvailableRAM < requiredMemory {
		return nil, fmt.Errorf("memoria insuficiente: se necesitan %s pero solo hay %s disponibles",
			formatBytes(requiredMemory), formatBytes(metrics.AvailableRAM))
	}

	fmt.Printf("=== Configuración del Benchmark ===\n")
	fmt.Printf("Tamaño de bloque: %d MB (%s)\n", config.BlockSizeMB, formatBytes(blockSize))
	fmt.Printf("Duración objetivo: %v\n", config.TargetDuration)
	fmt.Printf("RAM disponible: %s\n", formatBytes(metrics.AvailableRAM))
	fmt.Printf("RAM usada antes: %s (%.1f%%)\n", formatBytes(metrics.UsedRAM), metrics.UsagePercent)
	fmt.Println()

	fmt.Printf("Asignando %d MB de memoria...\n", config.BlockSizeMB)
	memoryBlock := make([]byte, blockSize)
	copyBlock := make([]byte, blockSize)

	// Asegurar limpieza de memoria al finalizar (incluso si hay errores)
	defer func() {
		// Limpiar explícitamente los bloques de memoria
		if memoryBlock != nil {
			for i := range memoryBlock {
				memoryBlock[i] = 0
			}
			memoryBlock = nil
		}
		if copyBlock != nil {
			for i := range copyBlock {
				copyBlock[i] = 0
			}
			copyBlock = nil
		}
		// Forzar garbage collection para liberar memoria inmediatamente
		runtime.GC()
		runtime.GC() // Segunda llamada para asegurar limpieza completa
	}()

	fmt.Println("Calentando memoria (warm-up)...")
	warmUpStart := time.Now()
	for i := 0; i < len(memoryBlock); i++ {
		memoryBlock[i] = byte(i % 255)
	}
	warmUpDuration := time.Since(warmUpStart)
	fmt.Printf("Warm-up completado en %v\n", warmUpDuration)
	fmt.Println()

	result := &RAMBenchmarkResult{
		BlockSize:       blockSize,
		Timestamp:       time.Now(),
		Iterations:      []BenchmarkIterationResult{},
		DataIntegrityOK: true,
	}

	fmt.Println("=== Iniciando Benchmark ===")
	fmt.Printf("Objetivo: ejecutar durante aproximadamente %v\n", config.TargetDuration)
	fmt.Println("------------------------------------------------")

	startTime := time.Now()
	iteration := 0
	var writeSpeeds []float64
	var readSpeeds []float64
	var copySpeeds []float64
	var latencies []float64

	result.FrequencyMHz = getRAMFrequencyWindows()
	result.MemoryChannels = getMemoryChannelsWindows()

	for {
		iteration++
		fmt.Printf("\nIteración %d:\n", iteration)

		writeStart := time.Now()
		expectedSum := int64(0)

		for i := 0; i < len(memoryBlock); i++ {
			val := byte(i % 255)
			memoryBlock[i] = val
			expectedSum += int64(val)
		}

		writeDuration := time.Since(writeStart)
		writeGBs := calculateGBPerSecond(blockSize, writeDuration)
		writeSpeeds = append(writeSpeeds, writeGBs)

		readStart := time.Now()
		var actualSum int64 = 0

		for i := 0; i < len(memoryBlock); i++ {
			actualSum += int64(memoryBlock[i])
		}

		readDuration := time.Since(readStart)
		readGBs := calculateGBPerSecond(blockSize, readDuration)
		readSpeeds = append(readSpeeds, readGBs)

		copyStart := time.Now()
		copy(copyBlock, memoryBlock)
		copyDuration := time.Since(copyStart)
		copyGBs := calculateGBPerSecond(blockSize, copyDuration)
		copySpeeds = append(copySpeeds, copyGBs)

		latencyStart := time.Now()
		latencyIterations := 100000
		var latencySum uint64 = 0
		for i := 0; i < latencyIterations; i++ {
			index := (i * 7919) % (len(memoryBlock) - 7)
			if index+7 < len(memoryBlock) {
				val := uint64(memoryBlock[index]) |
					uint64(memoryBlock[index+1])<<8 |
					uint64(memoryBlock[index+2])<<16 |
					uint64(memoryBlock[index+3])<<24 |
					uint64(memoryBlock[index+4])<<32 |
					uint64(memoryBlock[index+5])<<40 |
					uint64(memoryBlock[index+6])<<48 |
					uint64(memoryBlock[index+7])<<56
				latencySum += val
			}
		}
		latencyDuration := time.Since(latencyStart)
		latencyNs := float64(latencyDuration.Nanoseconds()) / float64(latencyIterations)
		latencies = append(latencies, latencyNs)
		_ = latencySum

		dataValid := actualSum == expectedSum
		if !dataValid {
			result.DataIntegrityOK = false
			fmt.Printf("   ADVERTENCIA: Error de integridad en iteración %d\n", iteration)
			fmt.Printf("    Suma esperada: %d, Suma obtenida: %d\n", expectedSum, actualSum)
		} else {
			fmt.Printf("   Integridad verificada\n")
		}

		iterResult := BenchmarkIterationResult{
			WriteGBs:  writeGBs,
			ReadGBs:   readGBs,
			CopyGBs:   copyGBs,
			LatencyNs: latencyNs,
			WriteTime: writeDuration,
			ReadTime:  readDuration,
			CopyTime:  copyDuration,
			DataValid: dataValid,
			Iteration: iteration,
		}
		result.Iterations = append(result.Iterations, iterResult)

		fmt.Printf("  Escritura: %.2f GB/s (%.2f MB/s) - %v\n",
			writeGBs, writeGBs*1024, writeDuration)
		fmt.Printf("  Lectura:   %.2f GB/s (%.2f MB/s) - %v\n",
			readGBs, readGBs*1024, readDuration)
		fmt.Printf("  Copia:     %.2f GB/s (%.2f MB/s) - %v\n",
			copyGBs, copyGBs*1024, copyDuration)
		fmt.Printf("  Latencia:  %.2f ns - %v\n",
			latencyNs, latencyDuration)

		elapsed := time.Since(startTime)
		if iteration >= config.MinIterations && elapsed >= config.TargetDuration {
			fmt.Printf("\nDuración objetivo alcanzada (%v)\n", elapsed)
			break
		}

		if iteration < config.MinIterations {
			fmt.Printf("  Progreso: %d/%d iteraciones mínimas\n", iteration, config.MinIterations)
		} else {
			remaining := config.TargetDuration - elapsed
			if remaining > 0 {
				fmt.Printf("  Tiempo transcurrido: %v / Objetivo: %v (restante: %v)\n",
					elapsed, config.TargetDuration, remaining)
			}
		}
	}

	result.TotalIterations = iteration
	result.TotalDuration = time.Since(startTime)

	if len(writeSpeeds) > 0 {
		result.SequentialWriteGBs = calculateAverage(writeSpeeds)
		result.SequentialWriteMBs = result.SequentialWriteGBs * 1024
		result.WriteMinGBs = calculateMin(writeSpeeds)
		result.WriteMaxGBs = calculateMax(writeSpeeds)
		result.WriteStdDev = calculateStdDev(writeSpeeds)
	}

	if len(readSpeeds) > 0 {
		result.SequentialReadGBs = calculateAverage(readSpeeds)
		result.SequentialReadMBs = result.SequentialReadGBs * 1024
		result.ReadMinGBs = calculateMin(readSpeeds)
		result.ReadMaxGBs = calculateMax(readSpeeds)
		result.ReadStdDev = calculateStdDev(readSpeeds)
	}

	if len(copySpeeds) > 0 {
		result.CopyGBs = calculateAverage(copySpeeds)
		result.CopyMBs = result.CopyGBs * 1024
		result.CopyMinGBs = calculateMin(copySpeeds)
		result.CopyMaxGBs = calculateMax(copySpeeds)
		result.CopyStdDev = calculateStdDev(copySpeeds)
	}

	if len(latencies) > 0 {
		result.LatencyNs = calculateAverage(latencies)
		result.LatencyMinNs = calculateMin(latencies)
		result.LatencyMaxNs = calculateMax(latencies)
		result.LatencyStdDev = calculateStdDev(latencies)
	}

	finalMetrics, err := GetRAMMetrics()
	if err == nil {
		result.RAMUsedBefore = metrics.UsedRAM
		result.RAMUsedAfter = finalMetrics.UsedRAM
		result.RAMUsageBefore = metrics.UsagePercent
		result.RAMUsageAfter = finalMetrics.UsagePercent
		result.RAMUsageChange = finalMetrics.UsagePercent - metrics.UsagePercent

		fmt.Printf("\n=== Verificación Post-Benchmark ===\n")
		fmt.Printf("RAM usada después: %s (%.1f%%)\n", formatBytes(finalMetrics.UsedRAM), finalMetrics.UsagePercent)
		fmt.Printf("Cambio en uso: %.1f%%\n", result.RAMUsageChange)
	}

	// La limpieza de memoria se realizará automáticamente mediante defer
	fmt.Println()
	return result, nil
}

func calculateGBPerSecond(sizeBytes uint64, duration time.Duration) float64 {
	seconds := duration.Seconds()
	if seconds == 0 {
		return 0
	}
	gbSize := float64(sizeBytes) / (1024 * 1024 * 1024)
	return gbSize / seconds
}

func printBenchmarkResult(name string, duration time.Duration, gbPerSec float64) {
	fmt.Printf("%s: %.2f GB/s (%.2f MB/s) - took %v\n", name, gbPerSec, gbPerSec*1024, duration)
}

func RunRAMBenchmarkWithDefault() (*RAMBenchmarkResult, error) {
	return RunRAMBenchmark(DefaultRAMBenchmarkConfig())
}

func calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

func calculateMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func calculateStdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	avg := calculateAverage(values)
	sumSqDiff := 0.0
	for _, v := range values {
		diff := v - avg
		sumSqDiff += diff * diff
	}
	variance := sumSqDiff / float64(len(values))
	return math.Sqrt(variance)
}

func FormatBenchmarkResult(result *RAMBenchmarkResult) string {
	if result == nil {
		return "No benchmark results available"
	}

	var output string
	output += "=== RESULTADOS DEL BENCHMARK DE RAM ===\n"
	output += fmt.Sprintf("Tamaño de bloque: %s\n", formatBytes(result.BlockSize))
	output += fmt.Sprintf("Iteraciones totales: %d\n", result.TotalIterations)
	output += fmt.Sprintf("Duración total: %v\n", result.TotalDuration)
	output += fmt.Sprintf("Timestamp: %s\n", result.Timestamp.Format(time.RFC3339))
	output += "\n"

	if result.FrequencyMHz > 0 {
		output += fmt.Sprintf("Frecuencia (Frequency / Speed): %d MHz\n", result.FrequencyMHz)
	}
	if result.MemoryChannels > 0 {
		output += fmt.Sprintf("Canales de Memoria (Channels): %d\n", result.MemoryChannels)
	}
	output += "\n"

	if result.DataIntegrityOK {
		output += " Integridad de datos: TODAS las iteraciones pasaron la verificación\n"
	} else {
		output += " Integridad de datos: Se detectaron errores en algunas iteraciones\n"
	}
	output += "\n"

	output += "=== READ SPEED (Velocidad de Lectura) ===\n"
	output += fmt.Sprintf("  Promedio:     %.2f GB/s (%.2f MB/s)\n", result.SequentialReadGBs, result.SequentialReadMBs)
	output += fmt.Sprintf("  Mínimo:       %.2f GB/s\n", result.ReadMinGBs)
	output += fmt.Sprintf("  Máximo:       %.2f GB/s\n", result.ReadMaxGBs)
	output += fmt.Sprintf("  Desv. Est.:   %.2f GB/s\n", result.ReadStdDev)
	if result.ReadStdDev > 0 {
		coeffVar := (result.ReadStdDev / result.SequentialReadGBs) * 100
		output += fmt.Sprintf("  Coef. Variación: %.2f%%\n", coeffVar)
		if coeffVar < 5 {
			output += "   Rendimiento muy estable\n"
		} else if coeffVar < 10 {
			output += "   Rendimiento estable\n"
		} else {
			output += "   Rendimiento variable (puede indicar interferencia del sistema)\n"
		}
	}
	output += "\n"

	output += "=== WRITE SPEED (Velocidad de Escritura) ===\n"
	output += fmt.Sprintf("  Promedio:     %.2f GB/s (%.2f MB/s)\n", result.SequentialWriteGBs, result.SequentialWriteMBs)
	output += fmt.Sprintf("  Mínimo:       %.2f GB/s\n", result.WriteMinGBs)
	output += fmt.Sprintf("  Máximo:       %.2f GB/s\n", result.WriteMaxGBs)
	output += fmt.Sprintf("  Desv. Est.:   %.2f GB/s\n", result.WriteStdDev)
	if result.WriteStdDev > 0 {
		coeffVar := (result.WriteStdDev / result.SequentialWriteGBs) * 100
		output += fmt.Sprintf("  Coef. Variación: %.2f%%\n", coeffVar)
		if coeffVar < 5 {
			output += "   Rendimiento muy estable\n"
		} else if coeffVar < 10 {
			output += "   Rendimiento estable\n"
		} else {
			output += "   Rendimiento variable (puede indicar interferencia del sistema)\n"
		}
	}
	output += "\n"

	output += "=== COPY SPEED (Velocidad de Copia) ===\n"
	if result.CopyGBs > 0 {
		output += fmt.Sprintf("  Promedio:     %.2f GB/s (%.2f MB/s)\n", result.CopyGBs, result.CopyMBs)
		output += fmt.Sprintf("  Mínimo:       %.2f GB/s\n", result.CopyMinGBs)
		output += fmt.Sprintf("  Máximo:       %.2f GB/s\n", result.CopyMaxGBs)
		output += fmt.Sprintf("  Desv. Est.:   %.2f GB/s\n", result.CopyStdDev)
		if result.CopyStdDev > 0 {
			coeffVar := (result.CopyStdDev / result.CopyGBs) * 100
			output += fmt.Sprintf("  Coef. Variación: %.2f%%\n", coeffVar)
		}
	} else {
		output += "  No disponible\n"
	}
	output += "\n"

	output += "=== LATENCY (Latencia) ===\n"
	if result.LatencyNs > 0 {
		output += fmt.Sprintf("  Promedio:     %.2f ns (%.3f ns)\n", result.LatencyNs, result.LatencyNs/1000.0)
		output += fmt.Sprintf("  Mínimo:       %.2f ns\n", result.LatencyMinNs)
		output += fmt.Sprintf("  Máximo:       %.2f ns\n", result.LatencyMaxNs)
		output += fmt.Sprintf("  Desv. Est.:   %.2f ns\n", result.LatencyStdDev)
	} else {
		output += "  No disponible\n"
	}
	output += "\n"

	if !result.DataIntegrityOK || result.TotalIterations <= 5 {
		output += "=== DETALLE POR ITERACIÓN ===\n"
		for _, iter := range result.Iterations {
			status := ""
			if !iter.DataValid {
				status = ""
			}
			output += fmt.Sprintf("  Iteración %d %s: Write=%.2f GB/s, Read=%.2f GB/s\n",
				iter.Iteration, status, iter.WriteGBs, iter.ReadGBs)
		}
		output += "\n"
	}

	output += "===============================================\n"
	if result.SequentialWriteGBs < 1.0 {
		output += "   Advertencia: Throughput de escritura muy bajo (< 1 GB/s)\n"
		output += "     Puede indicar problemas de hardware o interferencia del sistema\n"
	}
	if result.SequentialReadGBs < 1.0 {
		output += "   Advertencia: Throughput de lectura muy bajo (< 1 GB/s)\n"
		output += "     Puede indicar problemas de hardware o interferencia del sistema\n"
	}
	if result.SequentialReadGBs > result.SequentialWriteGBs*1.5 {
		output += "   Nota: La lectura es significativamente más rápida que la escritura\n"
		output += "     Esto es normal en la mayoría de sistemas\n"
	}
	if result.DataIntegrityOK && result.TotalIterations >= 3 {
		output += "  Benchmark completado exitosamente\n"
		output += "  Todas las verificaciones de integridad pasaron\n"
	}

	return output
}
