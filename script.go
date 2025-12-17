//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"v2/internal/baseline"
	"v2/internal/cpu"
	"v2/internal/disk"
	"v2/internal/ram"

	gopsutildisk "github.com/shirou/gopsutil/v4/disk"
)

// Estructuras para almacenar resultados de benchmarks
type BenchmarkSession struct {
	CPUResults   []*cpu.BenchmarkReport
	RAMResults   []*ram.RAMBenchmarkResult
	DiskResults  []*disk.Result
	SessionStart time.Time
}

var sessionResults = &BenchmarkSession{
	CPUResults:   make([]*cpu.BenchmarkReport, 0),
	RAMResults:   make([]*ram.RAMBenchmarkResult, 0),
	DiskResults:  make([]*disk.Result, 0),
	SessionStart: time.Now(),
}

func printHeader(text string) {
	fmt.Printf("\n%s\n%s\n", text, strings.Repeat("=", 60))
}
func printSection(text string) { fmt.Printf("\n%s\n", text) }
func printError(text string)   { fmt.Printf(" %s\n", text) }
func printInfo(text string)    { fmt.Printf(" %s\n", text) }

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func displaySystemInfo() {
	printHeader("INFORMACIÓN DEL SISTEMA")
	printSection("Procesador (CPU)")
	if info, err := cpu.GetCPUInfo(); err == nil {
		fmt.Println(info)
	} else {
		printError(fmt.Sprintf("Error obteniendo info CPU: %v", err))
	}
	if metrics, err := cpu.GetCPUMetrics(); err == nil {
		fmt.Printf("\nEstado Actual del CPU:\n  Uso: %.1f%% | Cores Físicos: %d | Threads: %d\n",
			metrics.UsagePercent, metrics.Cores, metrics.Threads)
		if metrics.ClockSpeed > 0 {
			fmt.Printf("  Frecuencia: %.2f MHz\n", metrics.ClockSpeed)
		}
		if metrics.Temperature > 0 {
			fmt.Printf("  Temperatura: %.1f°C\n", metrics.Temperature)
		}
		fmt.Println("   Uso del CPU: Porcentaje de capacidad utilizada (0-100%)\n    • <30%: Sistema inactivo\n    • 30-70%: Uso normal\n    • >70%: Sistema bajo carga")
	}
	printSection("Almacenamiento (Disco)")
	if info, err := disk.GetDiskInfo(); err == nil {
		fmt.Println(info)
	} else {
		printError(fmt.Sprintf("Error obteniendo info Disco: %v", err))
	}
	printSection("Entorno de Ejecución")
	fmt.Printf("Lenguaje: Go %s\nSistema Operativo: %s\nCPUs Disponibles: %d\n   Explicación:\n    • Go Version: Versión del compilador Go (afecta rendimiento)\n    • OS: Sistema operativo (Windows/Linux/macOS)\n    • CPUs: Número de CPUs que Go puede usar para paralelismo\n",
		runtime.Version(), runtime.GOOS, runtime.NumCPU())
}

func displayBaseline() {
	printHeader("BASELINE DEL SISTEMA")
	fmt.Print("El baseline establece el estado inicial del sistema para comparar\nel rendimiento durante los benchmarks y detectar anomalías.\n\n")
	if err := baseline.SetEnvironmentVariables(); err != nil {
		printError(fmt.Sprintf("Error estableciendo variables de entorno: %v", err))
	}
	result, err := baseline.RunBaseline()
	if err != nil {
		printError(fmt.Sprintf("Error ejecutando baseline: %v", err))
		return
	}
	printSection("Hardware Detectado")
	ramUsed := result.Hardware.RAMTotal - result.Baseline.MemoryAvailable
	diskUsed := result.Hardware.DiskTotal - result.Hardware.DiskFree
	fmt.Printf("Cores Físicos: %d | Cores Lógicos: %d\nRAM Total: %s | RAM Usada: %s | RAM Disponible: %s\nDisco Total: %s | Disco Usado: %s | Disco Libre: %s\n",
		result.Hardware.CPUCoresPhysical, result.Hardware.CPUCoresLogical,
		baseline.FormatBytes(result.Hardware.RAMTotal), baseline.FormatBytes(ramUsed), baseline.FormatBytes(result.Baseline.MemoryAvailable),
		baseline.FormatBytes(result.Hardware.DiskTotal), baseline.FormatBytes(diskUsed), baseline.FormatBytes(result.Hardware.DiskFree))
	if len(result.Hardware.NetworkInterfaces) > 0 {
		fmt.Printf("Interfaces de Red: %d\n", len(result.Hardware.NetworkInterfaces))
	}
	fmt.Println("   Hardware: Especificaciones físicas del sistema\n    • Cores Físicos: Núcleos reales del procesador\n    • Cores Lógicos: Incluye Hyper-Threading/SMT (mejor paralelismo)\n    • RAM Total: Memoria principal disponible\n    • Disco: Capacidad de almacenamiento permanente")
	printSection("Estado Inicial del Sistema (Baseline)")
	fmt.Printf("CPU Idle: %.1f%%\nMemoria Libre: %s | Disponible: %s\nDisco Libre: %s\n",
		result.Baseline.CPUIdlePercent,
		baseline.FormatBytes(result.Baseline.MemoryFree), baseline.FormatBytes(result.Baseline.MemoryAvailable),
		baseline.FormatBytes(result.Baseline.DiskFree))
	if result.Baseline.NetworkLatency > 0 {
		fmt.Printf("Latencia de Red Base: %v\n", result.Baseline.NetworkLatency)
	}
	fmt.Println("   Baseline: Estado del sistema antes de los tests")
	printSection("Configuración del Entorno")
	fmt.Printf("Sistema: %s %s | Go: %s\nProcesos Activos: %d\n   Entorno: Configuración del sistema operativo y runtime\n    • OS/Arquitectura: Plataforma de ejecución\n    • Go Version: Versión del runtime Go\n    • Procesos: Número de procesos en ejecución\n",
		result.Environment.OS, result.Environment.Architecture, result.Environment.GoVersion, result.Environment.ProcessCount)

	// Procesos de alto consumo
	if len(result.Environment.HighCPUProcesses) > 0 {
		sort.Slice(result.Environment.HighCPUProcesses, func(i, j int) bool {
			return result.Environment.HighCPUProcesses[i].CPUPercent > result.Environment.HighCPUProcesses[j].CPUPercent
		})
		fmt.Printf("\nProcesos con Alto Uso de CPU (>5%%):\n   Estos procesos pueden afectar el rendimiento de los benchmarks\n")
		for i, p := range result.Environment.HighCPUProcesses {
			if i >= 10 {
				break
			}
			fmt.Printf("  • %s (PID: %d) - %.1f%% CPU\n", p.Name, p.PID, p.CPUPercent)
		}
	}
	if len(result.Environment.HighMemoryProcesses) > 0 {
		sort.Slice(result.Environment.HighMemoryProcesses, func(i, j int) bool {
			return result.Environment.HighMemoryProcesses[i].MemoryMB > result.Environment.HighMemoryProcesses[j].MemoryMB
		})
		fmt.Printf("\nProcesos con Alto Uso de Memoria (>100MB):\n   Estos procesos consumen memoria que podría estar disponible\n")
		for i, p := range result.Environment.HighMemoryProcesses {
			if i >= 10 {
				break
			}
			fmt.Printf("  • %s (PID: %d) - %.1f MB\n", p.Name, p.PID, p.MemoryMB)
		}
	}

	if len(result.Environment.Recommendations) > 0 {
		fmt.Printf("\nRecomendaciones:\n")
		for _, rec := range result.Environment.Recommendations {
			fmt.Printf("  • %s\n", rec)
		}
	}
	fmt.Printf("\nNota: Compara estos valores con los resultados de los benchmarks\n       para detectar degradación del rendimiento del sistema.\n")
}

func runBenchmark(pattern, desc string) (string, *cpu.CPUStats, error) {
	printInfo(desc)
	done := make(chan struct{})
	statsChan := make(chan *cpu.CPUStats, 1)

	go func() {
		statsChan <- cpu.MonitorCPU(done)
	}()

	cmd := exec.Command("go", "test", "-bench", pattern, "-benchmem", "-benchtime=3s", "./internal/cpu")
	output, err := cmd.CombinedOutput()
	close(done)

	var stats *cpu.CPUStats
	select {
	case stats = <-statsChan:
	case <-time.After(2 * time.Second):
		stats = &cpu.CPUStats{}
	}

	if err != nil {
		return "", stats, fmt.Errorf("%v\n%s", err, string(output))
	}
	return string(output), stats, nil
}

func displaySummary(output string, stats *cpu.CPUStats) {
	if report, err := cpu.GenerateBenchmarkReport(output, stats); err != nil {
		printError(fmt.Sprintf("Error generando reporte: %v", err))
	} else {
		cpu.DisplayReport(report, printHeader, printSection)
		// Guardar resultado en la sesión
		sessionResults.CPUResults = append(sessionResults.CPUResults, report)
	}
}

func runRAMBenchmarkWithConfig(config ram.RAMBenchmarkConfig) {
	printHeader("BENCHMARK DE RAM")
	// Usar la función original sin barra de progreso
	if result, err := ram.RunRAMBenchmark(config); err != nil {
		printError(fmt.Sprintf("Error ejecutando benchmark: %v", err))
	} else {
		fmt.Printf("\n%s\n", ram.FormatBenchmarkResult(result))
		// Guardar resultado en la sesión
		sessionResults.RAMResults = append(sessionResults.RAMResults, result)
	}
}

func handleRAMStressMenu() {
	fmt.Println("\n=== Benchmark de RAM - Throughput (Ancho de Banda) ===\nEste benchmark mide la velocidad de lectura y escritura secuencial de RAM\nusando bloques grandes para evitar el cache L3 del CPU.\n\nCaracterísticas:\n  • Duración: ~1 minuto (múltiples iteraciones)\n  • Verificación de integridad de datos en cada iteración\n  • Estadísticas detalladas (promedio, min, max, desviación estándar)\n  • Análisis de estabilidad del rendimiento\n\nOpciones:\n1. Benchmark estándar (512 MB - recomendado, ~1 minuto)\n2. Benchmark rápido (256 MB, ~1 minuto)\n3. Benchmark medio (1 GB, ~1 minuto)\n4. Benchmark intensivo (2 GB, ~1 minuto)\n5. Tamaño personalizado (ingresar manualmente)\n\nOpción: ")
	var choice string
	fmt.Scanln(&choice)
	configs := map[string]struct {
		sizeMB int
		name   string
	}{
		"1": {512, "ESTÁNDAR"},
		"2": {256, "RÁPIDO"},
		"3": {1024, "MEDIO"},
		"4": {2048, "INTENSIVO"},
	}
	if choice == "5" {
		// Opción de tamaño personalizado
		fmt.Print("Ingrese el tamaño en MB (mínimo 64 MB): ")
		var sizeInput string
		fmt.Scanln(&sizeInput)
		sizeMB, err := strconv.Atoi(strings.TrimSpace(sizeInput))
		if err != nil || sizeMB < 64 {
			printError("Tamaño inválido. Debe ser un número entero mayor o igual a 64 MB.")
			return
		}
		printInfo(fmt.Sprintf("Configuración personalizada: %d MB, duración objetivo ~1 minuto\n", sizeMB))
		runRAMBenchmarkWithConfig(ram.RAMBenchmarkConfig{
			BlockSizeMB: sizeMB, TargetDuration: 60 * time.Second, MinIterations: 3})
	} else if cfg, ok := configs[choice]; ok {
		if choice == "1" {
			printHeader("BENCHMARK DE RAM - " + cfg.name)
			printInfo("Configuración: 512 MB, duración objetivo ~1 minuto\nEl benchmark ejecutará múltiples iteraciones hasta alcanzar 1 minuto\n")
			// Usar la función original sin barra de progreso
			defaultConfig := ram.DefaultRAMBenchmarkConfig()
			if result, err := ram.RunRAMBenchmark(defaultConfig); err != nil {
				printError(fmt.Sprintf("Error: %v", err))
			} else {
				fmt.Println(ram.FormatBenchmarkResult(result))
				// Guardar resultado en la sesión
				sessionResults.RAMResults = append(sessionResults.RAMResults, result)
			}
		} else {
			printInfo(fmt.Sprintf("Configuración: %d MB, duración objetivo ~1 minuto\n", cfg.sizeMB))
			runRAMBenchmarkWithConfig(ram.RAMBenchmarkConfig{
				BlockSizeMB: cfg.sizeMB, TargetDuration: 60 * time.Second, MinIterations: 3})
		}
	} else {
		printError("Opción inválida")
	}
}

func handleCPUMenu() {
	fmt.Println("\n1. Todas las pruebas")
	fmt.Println("2. Pruebas individuales")
	fmt.Print("Opción: ")

	var choice string
	fmt.Scanln(&choice)

	switch choice {
	case "1":
		output, stats, err := runBenchmark("^Benchmark.*", "Ejecutando todos las pruebas...")
		if err != nil {
			printError(fmt.Sprintf("Error: %v", err))
			return
		}
		displaySummary(output, stats)

	case "2":
		benchmarks := []string{
			"BenchmarkSHA256SingleCore",
			"BenchmarkAES256SingleCore",
			"BenchmarkSHA256MultiCore",
			"BenchmarkAES256MultiCore",
		}
		var allOutput strings.Builder
		var allStats []*cpu.CPUStats

		for _, name := range benchmarks {
			output, stats, err := runBenchmark("^"+name+"$", name+"...")
			if err != nil {
				printError(fmt.Sprintf("Error: %v", err))
				continue
			}
			allOutput.WriteString(output)
			allOutput.WriteString("\n")
			if stats != nil {
				allStats = append(allStats, stats)
			}
		}

		// Combinar estadísticas
		combinedStats := &cpu.CPUStats{Min: 100.0, TemperatureMin: 1000.0, ClockSpeedMin: 100000.0}
		var totalUsage, totalTemp, totalClock float64
		var totalSamples, tempSamples, clockSamples int
		for _, s := range allStats {
			if s == nil || s.Samples == 0 {
				continue
			}
			if s.Min < combinedStats.Min {
				combinedStats.Min = s.Min
			}
			if s.Max > combinedStats.Max {
				combinedStats.Max = s.Max
			}
			totalUsage += s.Average * float64(s.Samples)
			totalSamples += s.Samples
			if s.TemperatureAvg > 0 {
				totalTemp += s.TemperatureAvg
				tempSamples++
				if s.TemperatureMin < combinedStats.TemperatureMin {
					combinedStats.TemperatureMin = s.TemperatureMin
				}
				if s.TemperatureMax > combinedStats.TemperatureMax {
					combinedStats.TemperatureMax = s.TemperatureMax
				}
			}
			if s.ClockSpeedAvg > 0 {
				totalClock += s.ClockSpeedAvg
				clockSamples++
				if s.ClockSpeedMin < combinedStats.ClockSpeedMin {
					combinedStats.ClockSpeedMin = s.ClockSpeedMin
				}
				if s.ClockSpeedMax > combinedStats.ClockSpeedMax {
					combinedStats.ClockSpeedMax = s.ClockSpeedMax
				}
			}
			if s.EnergyConsumption > 0 {
				combinedStats.EnergyConsumption += s.EnergyConsumption
			}
			if s.StartTime.IsZero() || combinedStats.StartTime.IsZero() || s.StartTime.Before(combinedStats.StartTime) {
				combinedStats.StartTime = s.StartTime
			}
			if s.EndTime.After(combinedStats.EndTime) {
				combinedStats.EndTime = s.EndTime
			}
		}
		if totalSamples > 0 {
			combinedStats.Average, combinedStats.Samples = totalUsage/float64(totalSamples), totalSamples
		}
		if tempSamples > 0 {
			combinedStats.TemperatureAvg = totalTemp / float64(tempSamples)
		}
		if clockSamples > 0 {
			combinedStats.ClockSpeedAvg = totalClock / float64(clockSamples)
		}
		if len(allStats) > 0 {
			combinedStats.EnergyConsumption /= float64(len(allStats))
			combinedStats.Duration = combinedStats.EndTime.Sub(combinedStats.StartTime)
		}

		displaySummary(allOutput.String(), combinedStats)
	}
}

func handleDiskMenu() {
	handleDiskBenchmarkMenu()
}

func handleDiskBenchmarkMenu() {
	fmt.Print("\n=== Benchmark de Disco Basado en Archivos ===\nEste benchmark fuerza el disco físico mediante escritura y lectura continua\n\n")
	diskBefore, _ := getDiskFreeSpace()
	if diskBefore > 0 {
		printInfo(fmt.Sprintf("Espacio libre en disco antes: %s", formatBytes(diskBefore)))
	}
	tempDir := filepath.Join(os.TempDir(), "disk_benchmark")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		printError(fmt.Sprintf("Error creando directorio temporal: %v", err))
		return
	}
	fmt.Print("Tamaño total de archivos en GiB (1.0): ")
	var sizeInput string
	fmt.Scanln(&sizeInput)
	aggregateSizeGiB := 1.0
	if sizeInput != "" {
		if val, err := strconv.ParseFloat(sizeInput, 64); err == nil && val > 0 {
			aggregateSizeGiB = val
		}
	}
	bm := disk.NewMark(tempDir, aggregateSizeGiB)
	defer cleanupDiskBenchmark(bm, tempDir, diskBefore)
	if err := bm.SetTempDir(tempDir); err != nil {
		printError(fmt.Sprintf("Error configurando directorio: %v", err))
		return
	}
	printInfo("Generando datos aleatorios...")
	if err := bm.CreateRandomBlock(); err != nil {
		printError(fmt.Sprintf("Error generando bloque aleatorio: %v", err))
		return
	}
	bm.ClearBufferCacheEveryThreeSeconds()
	defer bm.StopCacheClear()
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Ejecutando test de escritura secuencial...", bm.RunSequentialWriteTest},
		{"Ejecutando test de lectura secuencial...", bm.RunSequentialReadTest},
		{"Ejecutando test de IOPS...", bm.RunIOPSTest},
		{"Ejecutando test de eliminación...", bm.RunDeleteTest},
	}
	for _, test := range tests {
		printInfo(test.name)
		if err := test.fn(); err != nil {
			printError(fmt.Sprintf("Error en %s: %v", test.name, err))
			if test.name != "Ejecutando test de eliminación..." {
				return
			}
		}
	}
	if result := bm.GetLastResult(); result != nil {
		disk.DisplayDiskBenchmarkResult(result, formatBytes, printHeader, printError)
		// Guardar resultado en la sesión
		sessionResults.DiskResults = append(sessionResults.DiskResults, result)
	}
	if err := bm.CleanupTestFiles(); err != nil {
		printError(fmt.Sprintf("Advertencia: error limpiando archivos: %v", err))
	} else if count, err := bm.VerifyCleanup(); err == nil && count == 0 {
		printInfo(" Archivos de test eliminados correctamente")
	}
}

func getDiskFreeSpace() (uint64, error) {
	usage, err := gopsutildisk.Usage(os.TempDir())
	if err != nil {
		return 0, err
	}
	return usage.Free, nil
}

func cleanupDiskBenchmark(bm *disk.Mark, tempDir string, diskBefore uint64) {
	printInfo("Limpiando archivos temporales...")
	if err := bm.CleanupTestFiles(); err != nil {
		printError(fmt.Sprintf("Error limpiando archivos: %v", err))
	}
	if err := os.RemoveAll(tempDir); err != nil {
		printError(fmt.Sprintf("Error eliminando directorio temporal: %v\nDirectorio que no se pudo eliminar: %s", err, tempDir))
	} else {
		printInfo(" Directorio temporal eliminado completamente")
	}
	printInfo("Esperando actualización del sistema de archivos...")
	time.Sleep(2 * time.Second)
	var diskAfter uint64
	var errAfter error
	for i := 0; i < 5; i++ {
		diskAfter, errAfter = getDiskFreeSpace()
		if errAfter == nil && i < 4 {
			time.Sleep(500 * time.Millisecond)
			if newDiskAfter, newErr := getDiskFreeSpace(); newErr == nil {
				if newDiskAfter == diskAfter {
					break
				}
				diskAfter = newDiskAfter
			}
		}
	}
	if errAfter == nil && diskBefore > 0 {
		if spaceFreed := diskAfter - diskBefore; spaceFreed > 0 {
			printInfo(fmt.Sprintf("Espacio liberado: %s", formatBytes(spaceFreed)))
		}
		printInfo(fmt.Sprintf("Espacio libre en disco después: %s", formatBytes(diskAfter)))
		diff := diskBefore - diskAfter
		if diff < 100*1024*1024 {
			if diskAfter >= diskBefore {
				printInfo(" Espacio en disco restaurado correctamente")
			} else {
				printInfo(fmt.Sprintf(" Espacio en disco restaurado (diferencia menor: %s, puede ser normal en Windows)", formatBytes(diff)))
			}
		} else {
			printError(fmt.Sprintf("Advertencia: El espacio en disco es menor que antes (diferencia: %s)", formatBytes(diff)))
			printInfo("   Nota: En Windows, el espacio puede tardar en actualizarse. Verifica manualmente si es necesario.")
		}
	}
}

func displayBenchmarkSummary() {
	printHeader("RESUMEN DE BENCHMARKS DE LA SESIÓN")

	totalBenchmarks := len(sessionResults.CPUResults) + len(sessionResults.RAMResults) + len(sessionResults.DiskResults)

	if totalBenchmarks == 0 {
		printInfo("No se han ejecutado benchmarks en esta sesión.")
		fmt.Printf("Sesión iniciada: %s\n", sessionResults.SessionStart.Format("2006-01-02 15:04:05"))
		return
	}

	fmt.Printf("Sesión iniciada: %s\n", sessionResults.SessionStart.Format("2006-01-02 15:04:05"))
	fmt.Printf("Total de benchmarks ejecutados: %d\n\n", totalBenchmarks)

	// Resumen de CPU
	if len(sessionResults.CPUResults) > 0 {
		printSection("BENCHMARKS DE CPU")
		fmt.Printf("Total ejecutados: %d\n\n", len(sessionResults.CPUResults))
		for i, report := range sessionResults.CPUResults {
			fmt.Printf("--- Ejecución %d ---\n", i+1)
			if report.Stats != nil && !report.Stats.StartTime.IsZero() {
				fmt.Printf("Fecha: %s\n", report.Stats.StartTime.Format("2006-01-02 15:04:05"))
			}
			if len(report.Benchmarks) > 0 {
				fmt.Println("Resultados:")
				for name, result := range report.Benchmarks {
					fmt.Printf("  %s: %s %s\n", name, result.Iterations, cpu.FormatTime(result.TimePerOp))
				}
			}
			if report.SingleCoreScore > 0 {
				fmt.Printf("Puntuación Single-Core: %.2f\n", report.SingleCoreScore)
			}
			if report.MultiCoreScore > 0 {
				fmt.Printf("Puntuación Multi-Core: %.2f\n", report.MultiCoreScore)
			}
			if report.Stats != nil {
				fmt.Printf("Uso CPU promedio: %.1f%% (Min: %.1f%%, Max: %.1f%%)\n",
					report.Stats.Average, report.Stats.Min, report.Stats.Max)
				if report.Stats.TemperatureAvg > 0 {
					fmt.Printf("Temperatura promedio: %.1f°C\n", report.Stats.TemperatureAvg)
				}
				if report.Stats.Duration > 0 {
					fmt.Printf("Duración: %v\n", report.Stats.Duration)
				}
			}
			fmt.Println()
		}
	}

	// Resumen de RAM
	if len(sessionResults.RAMResults) > 0 {
		printSection("BENCHMARKS DE RAM")
		fmt.Printf("Total ejecutados: %d\n\n", len(sessionResults.RAMResults))
		for i, result := range sessionResults.RAMResults {
			fmt.Printf("--- Ejecución %d ---\n", i+1)
			fmt.Printf("Fecha: %s\n", result.Timestamp.Format("2006-01-02 15:04:05"))
			fmt.Printf("Tamaño de bloque: %s\n", formatBytes(result.BlockSize))
			fmt.Printf("Iteraciones: %d | Duración: %v\n", result.TotalIterations, result.TotalDuration)
			fmt.Printf("Escritura: %.2f GB/s (%.2f MB/s)\n", result.SequentialWriteGBs, result.SequentialWriteMBs)
			fmt.Printf("Lectura: %.2f GB/s (%.2f MB/s)\n", result.SequentialReadGBs, result.SequentialReadMBs)
			fmt.Printf("Copia: %.2f GB/s (%.2f MB/s)\n", result.CopyGBs, result.CopyMBs)
			fmt.Printf("Latencia: %.2f ns\n", result.LatencyNs)
			if result.FrequencyMHz > 0 {
				fmt.Printf("Frecuencia RAM: %d MHz\n", result.FrequencyMHz)
			}
			if result.MemoryChannels > 0 {
				fmt.Printf("Canales de memoria: %d\n", result.MemoryChannels)
			}
			fmt.Println()
		}
	}

	// Resumen de Disco
	if len(sessionResults.DiskResults) > 0 {
		printSection("BENCHMARKS DE DISCO")
		fmt.Printf("Total ejecutados: %d\n\n", len(sessionResults.DiskResults))
		for i, result := range sessionResults.DiskResults {
			fmt.Printf("--- Ejecución %d ---\n", i+1)
			fmt.Printf("Fecha: %s\n", result.Start.Format("2006-01-02 15:04:05"))
			fmt.Printf("Escritura: %.2f MB/s (%.0f bytes en %v)\n",
				result.WriteThroughputMBs, float64(result.WrittenBytes), result.WrittenDuration)
			fmt.Printf("Lectura: %.2f MB/s (%.0f bytes en %v)\n",
				result.ReadThroughputMBs, float64(result.ReadBytes), result.ReadDuration)
			if result.SustainedThroughputMBs > 0 {
				fmt.Printf("Throughput sostenido: %.2f MB/s\n", result.SustainedThroughputMBs)
			}
			if result.IOOperations > 0 && result.IODuration > 0 {
				iops := float64(result.IOOperations) / result.IODuration.Seconds()
				fmt.Printf("IOPS: %.0f\n", iops)
			}
			if result.CPUAverageUsage > 0 {
				fmt.Printf("Uso CPU promedio: %.1f%%\n", result.CPUAverageUsage)
			}
			fmt.Println()
		}
	}

	printSection("ESTADÍSTICAS GENERALES")
	fmt.Printf("Benchmarks CPU: %d\n", len(sessionResults.CPUResults))
	fmt.Printf("Benchmarks RAM: %d\n", len(sessionResults.RAMResults))
	fmt.Printf("Benchmarks Disco: %d\n", len(sessionResults.DiskResults))
	fmt.Printf("Tiempo total de sesión: %v\n", time.Since(sessionResults.SessionStart))
}

func main() {
	printHeader("SURTUR FORGE - BENCHMARK")
	fmt.Printf("Fecha: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Print("¿Ejecutar baseline? (s/n): ")
	var runBaseline string
	fmt.Scanln(&runBaseline)
	if strings.ToLower(runBaseline) == "s" || strings.ToLower(runBaseline) == "y" {
		displayBaseline()
	} else {
		displaySystemInfo()
	}
	menu := map[string]func(){
		"1": handleCPUMenu,
		"2": handleDiskMenu,
		"3": handleRAMStressMenu,
		"4": displaySystemInfo,
		"5": displayBaseline,
		"6": displayBenchmarkSummary,
	}
	for {
		fmt.Printf("\n%s\n1. Benchmarks CPU\n2. Test Disco\n3. Test RAM\n4. Info Sistema\n5. Baseline\n6. Resumen de Benchmarks\n7. Salir\n\nOpción: ", strings.Repeat("-", 40))
		var choice string
		fmt.Scanln(&choice)
		if choice == "7" {
			fmt.Println("\nSaliendo...")
			os.Exit(0)
		}
		if fn, ok := menu[choice]; ok {
			fn()
		} else {
			printError("Opción inválida")
		}
	}
}
