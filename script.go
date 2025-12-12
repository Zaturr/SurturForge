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
		fmt.Println("   Explicación:\n    • Modelo: Nombre del procesador según el fabricante\n    • Cores Físicos: Núcleos reales del procesador (rendimiento base)\n    • Cores Lógicos: Cores físicos × threads por core (Hyper-Threading/SMT)\n    • Frecuencia: Velocidad del reloj (MHz) - mayor = más rápido\n    • Cache: Memoria rápida integrada en el CPU (reduce latencia)")
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
		fmt.Println("   Explicación:\n    • Tamaño Total: Capacidad total del dispositivo\n    • Espacio Usado: Cantidad de datos almacenados\n    • Espacio Libre: Espacio disponible para nuevos datos\n    • Uso: Porcentaje del disco ocupado\n    • Inodos: Estructuras de metadatos del sistema de archivos\n      (Linux/Unix: límite de archivos que se pueden crear)")
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
	fmt.Printf("Cores Físicos: %d | Cores Lógicos: %d\nRAM Total: %s\nDisco Total: %s | Disco Libre: %s\n",
		result.Hardware.CPUCoresPhysical, result.Hardware.CPUCoresLogical,
		baseline.FormatBytes(result.Hardware.RAMTotal),
		baseline.FormatBytes(result.Hardware.DiskTotal), baseline.FormatBytes(result.Hardware.DiskFree))
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
	fmt.Println("   Baseline: Estado del sistema antes de los tests\n    • CPU Idle: Porcentaje de CPU sin uso (mayor = sistema más libre)\n    • Memoria Libre: RAM no utilizada\n    • Memoria Disponible: RAM que puede usarse (incluye caché liberable)\n    • Disco Libre: Espacio disponible para operaciones de I/O\n    • Latencia de Red: Tiempo de respuesta de red base")
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

func runBenchmark(pattern, desc string) (string, *cpu.CPUStats, float64, error) {
	// Capturar CPU antes del benchmark
	var cpuUsageBefore float64
	if metrics, err := cpu.GetCPUMetrics(); err == nil {
		cpuUsageBefore = metrics.UsagePercent
	}

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
		return "", stats, cpuUsageBefore, fmt.Errorf("%v\n%s", err, string(output))
	}
	return string(output), stats, cpuUsageBefore, nil
}

func displaySummary(output string, stats *cpu.CPUStats, cpuUsageBefore float64) {
	if report, err := cpu.GenerateBenchmarkReport(output, stats, cpuUsageBefore); err != nil {
		printError(fmt.Sprintf("Error generando reporte: %v", err))
	} else {
		cpu.DisplayReport(report, printHeader, printSection)
		cpu.DisplayCPUBenchmarkSummary(report)
	}
}

func runRAMBenchmarkWithConfig(config ram.RAMBenchmarkConfig) {
	printHeader("BENCHMARK DE RAM")
	if result, err := ram.RunRAMBenchmark(config); err != nil {
		printError(fmt.Sprintf("Error ejecutando benchmark: %v", err))
	} else {
		fmt.Printf("\n%s\n", ram.FormatBenchmarkResult(result))
		ram.DisplayRAMBenchmarkSummary(result)
	}
}

func handleRAMStressMenu() {
	fmt.Println("\n=== Benchmark de RAM - Throughput (Ancho de Banda) ===\nEste benchmark mide la velocidad de lectura y escritura secuencial de RAM\nusando bloques grandes para evitar el cache L3 del CPU.\n\nCaracterísticas:\n  • Duración: ~1 minuto (múltiples iteraciones)\n  • Verificación de integridad de datos en cada iteración\n  • Estadísticas detalladas (promedio, min, max, desviación estándar)\n  • Análisis de estabilidad del rendimiento\n\nOpciones:\n1. Benchmark estándar (512 MB - recomendado, ~1 minuto)\n2. Benchmark rápido (256 MB, ~1 minuto)\n3. Benchmark medio (1 GB, ~1 minuto)\n4. Benchmark intensivo (2 GB, ~1 minuto)\n\nOpción: ")
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
	if cfg, ok := configs[choice]; ok {
		if choice == "1" {
			printHeader("BENCHMARK DE RAM - " + cfg.name)
			printInfo("Configuración: 512 MB, duración objetivo ~1 minuto\nEl benchmark ejecutará múltiples iteraciones hasta alcanzar 1 minuto\n")
			if result, err := ram.RunRAMBenchmarkWithDefault(); err != nil {
				printError(fmt.Sprintf("Error: %v", err))
			} else {
				fmt.Println(ram.FormatBenchmarkResult(result))
				ram.DisplayRAMBenchmarkSummary(result)
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
	fmt.Println("\n1. Todos los benchmarks")
	fmt.Println("2. Benchmarks individuales")
	fmt.Print("Opción: ")

	var choice string
	fmt.Scanln(&choice)

	switch choice {
	case "1":
		output, stats, cpuBefore, err := runBenchmark("^Benchmark.*", "Ejecutando todos los benchmarks...")
		if err != nil {
			printError(fmt.Sprintf("Error: %v", err))
			return
		}
		displaySummary(output, stats, cpuBefore)

	case "2":
		benchmarks := []string{
			"BenchmarkSHA256SingleCore",
			"BenchmarkAES256SingleCore",
			"BenchmarkSHA256MultiCore",
			"BenchmarkAES256MultiCore",
		}
		var allOutput strings.Builder
		var allStats []*cpu.CPUStats

		var cpuBefore float64
		for i, name := range benchmarks {
			output, stats, cpuBeforeTemp, err := runBenchmark("^"+name+"$", name+"...")
			if err != nil {
				printError(fmt.Sprintf("Error: %v", err))
				continue
			}
			if i == 0 {
				cpuBefore = cpuBeforeTemp // Usar el CPU antes del primer benchmark
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

		displaySummary(allOutput.String(), combinedStats, cpuBefore)
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
		disk.DisplayDiskBenchmarkSummary(result)
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
	}
	for {
		fmt.Printf("\n%s\n1. Benchmarks CPU\n2. Test Disco\n3. Test RAM\n4. Info Sistema\n5. Baseline\n6. Salir\n\nOpción: ", strings.Repeat("-", 40))
		var choice string
		fmt.Scanln(&choice)
		if choice == "6" {
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
