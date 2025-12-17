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

	// Estadísticas extendidas
	WriteStats               ExtendedStats
	ReadStats                ExtendedStats
	CopyStats                ExtendedStats
	LatencyStats             ExtendedStats
	WriteTemporalStability   TemporalStability
	ReadTemporalStability    TemporalStability
	CopyTemporalStability    TemporalStability
	LatencyTemporalStability TemporalStability
	Environment              EnvironmentInfo
	RunNumber                int
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

	// Obtener información del entorno
	result.Environment = getEnvironmentInfo()

	// Calcular estadísticas extendidas
	if len(writeSpeeds) > 0 {
		result.WriteStats = calculateExtendedStats(writeSpeeds)
		result.WriteTemporalStability = calculateTemporalStability(writeSpeeds)
	}

	if len(readSpeeds) > 0 {
		result.ReadStats = calculateExtendedStats(readSpeeds)
		result.ReadTemporalStability = calculateTemporalStability(readSpeeds)
	}

	if len(copySpeeds) > 0 {
		result.CopyStats = calculateExtendedStats(copySpeeds)
		result.CopyTemporalStability = calculateTemporalStability(copySpeeds)
	}

	if len(latencies) > 0 {
		result.LatencyStats = calculateExtendedStats(latencies)
		result.LatencyTemporalStability = calculateTemporalStability(latencies)
	}

	finalMetrics, err := GetRAMMetrics()
	if err == nil {
		fmt.Printf("\n=== Verificación Post-Benchmark ===\n")
		fmt.Printf("RAM usada después: %s (%.1f%%)\n", formatBytes(finalMetrics.UsedRAM), finalMetrics.UsagePercent)
		fmt.Printf("Cambio en uso: %.1f%%\n", finalMetrics.UsagePercent-metrics.UsagePercent)
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

// RunMultipleRAMBenchmarks ejecuta múltiples benchmarks completos
func RunMultipleRAMBenchmarks(config RAMBenchmarkConfig, numRuns int) ([]*RAMBenchmarkResult, error) {
	if numRuns < 1 {
		numRuns = 1
	}

	results := make([]*RAMBenchmarkResult, 0, numRuns)

	for i := 0; i < numRuns; i++ {
		fmt.Printf("\n=== Ejecución %d de %d ===\n", i+1, numRuns)

		result, err := RunRAMBenchmark(config)
		if err != nil {
			return results, fmt.Errorf("error en ejecución %d: %w", i+1, err)
		}

		result.RunNumber = i + 1
		results = append(results, result)

		// Pequeña pausa entre ejecuciones para estabilizar el sistema
		if i < numRuns-1 {
			time.Sleep(2 * time.Second)
		}
	}

	return results, nil
}

// AnalyzeMultipleRuns analiza y compara múltiples ejecuciones
func AnalyzeMultipleRuns(results []*RAMBenchmarkResult) string {
	if len(results) == 0 {
		return "No hay resultados para analizar"
	}

	var output string
	output += "=== ANÁLISIS DE MÚLTIPLES EJECUCIONES ===\n"
	output += fmt.Sprintf("Total de ejecuciones: %d\n\n", len(results))

	// Agregar estadísticas de cada ejecución
	for _, result := range results {
		output += fmt.Sprintf("--- Ejecución %d ---\n", result.RunNumber)
		output += FormatBenchmarkResult(result)
		output += "\n"
	}

	// Calcular promedios entre ejecuciones
	if len(results) > 1 {
		var writeAvgs, readAvgs, copyAvgs, latencyAvgs []float64

		for _, r := range results {
			writeAvgs = append(writeAvgs, r.SequentialWriteGBs)
			readAvgs = append(readAvgs, r.SequentialReadGBs)
			copyAvgs = append(copyAvgs, r.CopyGBs)
			latencyAvgs = append(latencyAvgs, r.LatencyNs)
		}

		output += "=== RESUMEN ENTRE EJECUCIONES ===\n"
		output += fmt.Sprintf("READ - Promedio: %.2f GB/s, StdDev: %.2f GB/s\n",
			calculateAverage(readAvgs), calculateStdDev(readAvgs))
		output += fmt.Sprintf("WRITE - Promedio: %.2f GB/s, StdDev: %.2f GB/s\n",
			calculateAverage(writeAvgs), calculateStdDev(writeAvgs))
		output += fmt.Sprintf("COPY - Promedio: %.2f GB/s, StdDev: %.2f GB/s\n",
			calculateAverage(copyAvgs), calculateStdDev(copyAvgs))
		output += fmt.Sprintf("LATENCY - Promedio: %.2f ns, StdDev: %.2f ns\n",
			calculateAverage(latencyAvgs), calculateStdDev(latencyAvgs))
	}

	return output
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

	// Información del entorno
	if result.Environment.GoVersion != "" {
		output += "\n=== INFORMACIÓN DEL ENTORNO ===\n"
		output += fmt.Sprintf("  Versión de Go: %s\n", result.Environment.GoVersion)
		output += fmt.Sprintf("  Sistema Operativo: %s\n", result.Environment.OS)
		output += fmt.Sprintf("  Arquitectura: %s\n", result.Environment.Arch)
		output += fmt.Sprintf("  Núcleos de CPU: %d\n", result.Environment.CPUCores)
		if result.Environment.SystemLoadAvg > 0 {
			output += fmt.Sprintf("  Carga del sistema: %.2f\n", result.Environment.SystemLoadAvg)
		}
		output += fmt.Sprintf("  Carga de memoria: %.1f%%\n", result.Environment.MemoryLoad)
		if result.Environment.CPUUsageBefore > 0 {
			output += fmt.Sprintf("  Uso de CPU antes: %.1f%%\n", result.Environment.CPUUsageBefore)
		}
		output += "\n"
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

	// Estadísticas extendidas de lectura
	if result.ReadStats.Median > 0 {
		output += "=== READ SPEED - ESTADÍSTICAS EXTENDIDAS ===\n"
		output += fmt.Sprintf("  Mediana (P50):   %.2f GB/s\n", result.ReadStats.Median)
		output += fmt.Sprintf("  Percentil 95:    %.2f GB/s\n", result.ReadStats.P95)
		output += fmt.Sprintf("  Percentil 99:    %.2f GB/s\n", result.ReadStats.P99)
		output += fmt.Sprintf("  IQR:             %.2f GB/s\n", result.ReadStats.IQR)
		output += fmt.Sprintf("  IC 95%%:          [%.2f, %.2f] GB/s\n",
			result.ReadStats.CI95Lower, result.ReadStats.CI95Upper)
		if len(result.ReadStats.Outliers) > 0 {
			output += fmt.Sprintf("  Outliers:        Iteraciones %v\n", result.ReadStats.Outliers)
		}
		output += "\n"
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

	// Estadísticas extendidas de escritura
	if result.WriteStats.Median > 0 {
		output += "=== WRITE SPEED - ESTADÍSTICAS EXTENDIDAS ===\n"
		output += fmt.Sprintf("  Mediana (P50):   %.2f GB/s\n", result.WriteStats.Median)
		output += fmt.Sprintf("  Percentil 95:    %.2f GB/s\n", result.WriteStats.P95)
		output += fmt.Sprintf("  Percentil 99:    %.2f GB/s\n", result.WriteStats.P99)
		output += fmt.Sprintf("  IQR:             %.2f GB/s\n", result.WriteStats.IQR)
		output += fmt.Sprintf("  IC 95%%:          [%.2f, %.2f] GB/s\n",
			result.WriteStats.CI95Lower, result.WriteStats.CI95Upper)
		if len(result.WriteStats.Outliers) > 0 {
			output += fmt.Sprintf("  Outliers:        Iteraciones %v\n", result.WriteStats.Outliers)
		}
		output += "\n"
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

	// Estadísticas extendidas de copia
	if result.CopyStats.Median > 0 {
		output += "=== COPY SPEED - ESTADÍSTICAS EXTENDIDAS ===\n"
		output += fmt.Sprintf("  Mediana (P50):   %.2f GB/s\n", result.CopyStats.Median)
		output += fmt.Sprintf("  Percentil 95:    %.2f GB/s\n", result.CopyStats.P95)
		output += fmt.Sprintf("  Percentil 99:    %.2f GB/s\n", result.CopyStats.P99)
		output += fmt.Sprintf("  IQR:             %.2f GB/s\n", result.CopyStats.IQR)
		output += fmt.Sprintf("  IC 95%%:          [%.2f, %.2f] GB/s\n",
			result.CopyStats.CI95Lower, result.CopyStats.CI95Upper)
		if len(result.CopyStats.Outliers) > 0 {
			output += fmt.Sprintf("  Outliers:        Iteraciones %v\n", result.CopyStats.Outliers)
		}
		output += "\n"
	}

	// Estadísticas extendidas de latencia
	if result.LatencyStats.Median > 0 {
		output += "=== LATENCY - ESTADÍSTICAS EXTENDIDAS ===\n"
		output += fmt.Sprintf("  Mediana (P50):   %.2f ns\n", result.LatencyStats.Median)
		output += fmt.Sprintf("  Percentil 95:    %.2f ns\n", result.LatencyStats.P95)
		output += fmt.Sprintf("  Percentil 99:    %.2f ns\n", result.LatencyStats.P99)
		output += fmt.Sprintf("  IQR:             %.2f ns\n", result.LatencyStats.IQR)
		output += fmt.Sprintf("  IC 95%%:          [%.2f, %.2f] ns\n",
			result.LatencyStats.CI95Lower, result.LatencyStats.CI95Upper)
		if len(result.LatencyStats.Outliers) > 0 {
			output += fmt.Sprintf("  Outliers:        Iteraciones %v\n", result.LatencyStats.Outliers)
		}
		output += "\n"
	}

	// Estabilidad temporal
	if result.ReadTemporalStability.StabilityScore > 0 {
		output += "=== ESTABILIDAD TEMPORAL ===\n"
		output += fmt.Sprintf("    Primera mitad:  %.2f GB/s\n", result.ReadTemporalStability.FirstHalfAvg)
		output += fmt.Sprintf("    Segunda mitad:  %.2f GB/s\n", result.ReadTemporalStability.SecondHalfAvg)
		output += fmt.Sprintf("    Tendencia:      %.4f (positiva = mejora)\n", result.ReadTemporalStability.TrendSlope)
		output += fmt.Sprintf("    Fuerza:         %.2f%%\n", result.ReadTemporalStability.TrendStrength*100)
		output += fmt.Sprintf("    Score:          %.1f/100\n", result.ReadTemporalStability.StabilityScore)

		if result.WriteTemporalStability.StabilityScore > 0 {
			output += "  WRITE:\n"
			output += fmt.Sprintf("    Primera mitad:  %.2f GB/s\n", result.WriteTemporalStability.FirstHalfAvg)
			output += fmt.Sprintf("    Segunda mitad:  %.2f GB/s\n", result.WriteTemporalStability.SecondHalfAvg)
			output += fmt.Sprintf("    Tendencia:      %.4f\n", result.WriteTemporalStability.TrendSlope)
			output += fmt.Sprintf("    Score:          %.1f/100\n", result.WriteTemporalStability.StabilityScore)
		}
		output += "\n"
	}

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
