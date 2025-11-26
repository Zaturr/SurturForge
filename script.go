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

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func printHeader(text string) {
	fmt.Printf("\n%s%s%s\n", colorBold+colorCyan, text, colorReset)
	fmt.Println(strings.Repeat("=", 60))
}

func printSection(text string) {
	fmt.Printf("\n%s%s%s\n", colorYellow, text, colorReset)
}

func printError(text string) {
	fmt.Printf("%s✗ %s%s\n", colorRed, text, colorReset)
}

func printInfo(text string) {
	fmt.Printf("%s %s%s\n", colorCyan, text, colorReset)
}

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

	// Información de CPU
	printSection("Procesador (CPU)")
	if info, err := cpu.GetCPUInfo(); err == nil {
		fmt.Println(info)
		fmt.Println("   Explicación:")
		fmt.Println("    • Modelo: Nombre del procesador según el fabricante")
		fmt.Println("    • Cores Físicos: Núcleos reales del procesador (rendimiento base)")
		fmt.Println("    • Cores Lógicos: Cores físicos × threads por core (Hyper-Threading/SMT)")
		fmt.Println("    • Frecuencia: Velocidad del reloj (MHz) - mayor = más rápido")
		fmt.Println("    • Cache: Memoria rápida integrada en el CPU (reduce latencia)")
	} else {
		printError(fmt.Sprintf("Error obteniendo info CPU: %v", err))
	}

	if metrics, err := cpu.GetCPUMetrics(); err == nil {
		fmt.Printf("\n%sEstado Actual del CPU:%s\n", colorBold, colorReset)
		fmt.Printf("  Uso: %.1f%% | Cores Físicos: %d | Threads: %d\n",
			metrics.UsagePercent, metrics.Cores, metrics.Threads)
		if metrics.ClockSpeed > 0 {
			fmt.Printf("  Frecuencia: %.2f MHz\n", metrics.ClockSpeed)
		}
		if metrics.Temperature > 0 {
			fmt.Printf("  Temperatura: %.1f°C\n", metrics.Temperature)
		}
		fmt.Println("   Uso del CPU: Porcentaje de capacidad utilizada (0-100%)")
		fmt.Println("    • <30%: Sistema inactivo")
		fmt.Println("    • 30-70%: Uso normal")
		fmt.Println("    • >70%: Sistema bajo carga")
	}

	// Información de Disco
	printSection("Almacenamiento (Disco)")
	if info, err := disk.GetDiskInfo(); err == nil {
		fmt.Println(info)
		fmt.Println("   Explicación:")
		fmt.Println("    • Tamaño Total: Capacidad total del dispositivo")
		fmt.Println("    • Espacio Usado: Cantidad de datos almacenados")
		fmt.Println("    • Espacio Libre: Espacio disponible para nuevos datos")
		fmt.Println("    • Uso: Porcentaje del disco ocupado")
		fmt.Println("    • Inodos: Estructuras de metadatos del sistema de archivos")
		fmt.Println("      (Linux/Unix: límite de archivos que se pueden crear)")
	} else {
		printError(fmt.Sprintf("Error obteniendo info Disco: %v", err))
	}

	// Información del Entorno
	printSection("Entorno de Ejecución")
	fmt.Printf("Lenguaje: Go %s\n", runtime.Version())
	fmt.Printf("Sistema Operativo: %s\n", runtime.GOOS)
	fmt.Printf("CPUs Disponibles: %d\n", runtime.NumCPU())
	fmt.Println("   Explicación:")
	fmt.Println("    • Go Version: Versión del compilador Go (afecta rendimiento)")
	fmt.Println("    • OS: Sistema operativo (Windows/Linux/macOS)")
	fmt.Println("    • CPUs: Número de CPUs que Go puede usar para paralelismo")
}

func displayBaseline() {
	printHeader("BASELINE DEL SISTEMA")
	fmt.Println("El baseline establece el estado inicial del sistema para comparar")
	fmt.Println("el rendimiento durante los benchmarks y detectar anomalías.")
	fmt.Println()

	if err := baseline.SetEnvironmentVariables(); err != nil {
		printError(fmt.Sprintf("Error estableciendo variables de entorno: %v", err))
	}

	result, err := baseline.RunBaseline()
	if err != nil {
		printError(fmt.Sprintf("Error ejecutando baseline: %v", err))
		return
	}

	// Hardware detectado
	printSection("Hardware Detectado")
	fmt.Printf("Cores Físicos: %d | Cores Lógicos: %d\n",
		result.Hardware.CPUCoresPhysical, result.Hardware.CPUCoresLogical)
	fmt.Printf("RAM Total: %s\n", baseline.FormatBytes(result.Hardware.RAMTotal))
	fmt.Printf("Disco Total: %s | Disco Libre: %s\n",
		baseline.FormatBytes(result.Hardware.DiskTotal),
		baseline.FormatBytes(result.Hardware.DiskFree))
	if len(result.Hardware.NetworkInterfaces) > 0 {
		fmt.Printf("Interfaces de Red: %d\n", len(result.Hardware.NetworkInterfaces))
	}
	fmt.Println("   Hardware: Especificaciones físicas del sistema")
	fmt.Println("    • Cores Físicos: Núcleos reales del procesador")
	fmt.Println("    • Cores Lógicos: Incluye Hyper-Threading/SMT (mejor paralelismo)")
	fmt.Println("    • RAM Total: Memoria principal disponible")
	fmt.Println("    • Disco: Capacidad de almacenamiento permanente")

	// Baseline del sistema
	printSection("Estado Inicial del Sistema (Baseline)")
	fmt.Printf("CPU Idle: %.1f%%\n", result.Baseline.CPUIdlePercent)
	fmt.Printf("Memoria Libre: %s | Disponible: %s\n",
		baseline.FormatBytes(result.Baseline.MemoryFree),
		baseline.FormatBytes(result.Baseline.MemoryAvailable))
	fmt.Printf("Disco Libre: %s\n", baseline.FormatBytes(result.Baseline.DiskFree))
	if result.Baseline.NetworkLatency > 0 {
		fmt.Printf("Latencia de Red Base: %v\n", result.Baseline.NetworkLatency)
	}
	fmt.Println("   Baseline: Estado del sistema antes de los tests")
	fmt.Println("    • CPU Idle: Porcentaje de CPU sin uso (mayor = sistema más libre)")
	fmt.Println("    • Memoria Libre: RAM no utilizada")
	fmt.Println("    • Memoria Disponible: RAM que puede usarse (incluye caché liberable)")
	fmt.Println("    • Disco Libre: Espacio disponible para operaciones de I/O")
	fmt.Println("    • Latencia de Red: Tiempo de respuesta de red base")

	// Entorno
	printSection("Configuración del Entorno")
	fmt.Printf("Sistema: %s %s | Go: %s\n",
		result.Environment.OS,
		result.Environment.Architecture,
		result.Environment.GoVersion)
	fmt.Printf("Procesos Activos: %d\n", result.Environment.ProcessCount)
	fmt.Println("   Entorno: Configuración del sistema operativo y runtime")
	fmt.Println("    • OS/Arquitectura: Plataforma de ejecución")
	fmt.Println("    • Go Version: Versión del runtime Go")
	fmt.Println("    • Procesos: Número de procesos en ejecución")

	// Procesos de alto consumo de CPU
	if len(result.Environment.HighCPUProcesses) > 0 {
		sort.Slice(result.Environment.HighCPUProcesses, func(i, j int) bool {
			return result.Environment.HighCPUProcesses[i].CPUPercent > result.Environment.HighCPUProcesses[j].CPUPercent
		})
		fmt.Printf("\n%sProcesos con Alto Uso de CPU (>5%%):%s\n", colorYellow, colorReset)
		fmt.Println("   Estos procesos pueden afectar el rendimiento de los benchmarks")
		for i, p := range result.Environment.HighCPUProcesses {
			if i >= 10 {
				break
			}
			fmt.Printf("  • %s (PID: %d) - %.1f%% CPU\n", p.Name, p.PID, p.CPUPercent)
		}
	}

	// Procesos de alto consumo de memoria
	if len(result.Environment.HighMemoryProcesses) > 0 {
		sort.Slice(result.Environment.HighMemoryProcesses, func(i, j int) bool {
			return result.Environment.HighMemoryProcesses[i].MemoryMB > result.Environment.HighMemoryProcesses[j].MemoryMB
		})
		fmt.Printf("\n%sProcesos con Alto Uso de Memoria (>100MB):%s\n", colorYellow, colorReset)
		fmt.Println("   Estos procesos consumen memoria que podría estar disponible")
		for i, p := range result.Environment.HighMemoryProcesses {
			if i >= 10 {
				break
			}
			fmt.Printf("  • %s (PID: %d) - %.1f MB\n", p.Name, p.PID, p.MemoryMB)
		}
	}

	// Recomendaciones
	if len(result.Environment.Recommendations) > 0 {
		fmt.Printf("\n%sRecomendaciones:%s\n", colorCyan, colorReset)
		for _, rec := range result.Environment.Recommendations {
			fmt.Printf("  • %s\n", rec)
		}
	}

	fmt.Printf("\n%sNota:%s Compara estos valores con los resultados de los benchmarks\n",
		colorCyan, colorReset)
	fmt.Println("       para detectar degradación del rendimiento del sistema.")
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
	report, err := cpu.GenerateBenchmarkReport(output, stats)
	if err != nil {
		printError(fmt.Sprintf("Error generando reporte: %v", err))
		return
	}

	cpu.DisplayReport(report, colorBold, colorReset, colorCyan, colorGreen, colorYellow, colorRed, printHeader, printSection)
}

func runRAMStressTest(intensity string) {
	config := ram.GetIntensityConfig(intensity)
	result, err := ram.RunRAMStress(config)
	if err != nil {
		printError(fmt.Sprintf("Error: %v", err))
		return
	}
	ram.DisplayRAMStressResult(result, colorReset, colorRed, printHeader, printError)
}

func runFullRAMEvaluation() {
	baselineResult, _ := baseline.RunBaseline()
	baselinePtr := &baselineResult
	ram.RunFullRAMEvaluationWithDisplay(baselinePtr, printHeader, printInfo, printError)
}

func handleRAMStressMenu() {
	fmt.Println("\n1. Evaluación completa. \n2. Ligero  \n3. Medio  \n4. Pesado \nOpción: ")
	var choice string
	fmt.Scanln(&choice)

	switch choice {
	case "1":
		runFullRAMEvaluation()
	case "2":
		runRAMStressTest("light")
	case "3":
		runRAMStressTest("medium")
	case "4":
		runRAMStressTest("heavy")
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
		output, stats, err := runBenchmark("^Benchmark.*", "Ejecutando todos los benchmarks...")
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
		combinedStats := &cpu.CPUStats{
			Min:            100.0,
			TemperatureMin: 1000.0,
			ClockSpeedMin:  100000.0,
		}
		var totalUsage, totalTemp, totalClock float64
		var totalSamples, tempSamples, clockSamples int
		for _, s := range allStats {
			if s != nil && s.Samples > 0 {
				if s.Min < combinedStats.Min {
					combinedStats.Min = s.Min
				}
				if s.Max > combinedStats.Max {
					combinedStats.Max = s.Max
				}
				totalUsage += s.Average * float64(s.Samples)
				totalSamples += s.Samples

				// Temperaturas
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

				// Frecuencias
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

				// Consumo energético (promedio)
				if s.EnergyConsumption > 0 {
					combinedStats.EnergyConsumption += s.EnergyConsumption
				}

				// Tiempos
				if s.StartTime.IsZero() || combinedStats.StartTime.IsZero() || s.StartTime.Before(combinedStats.StartTime) {
					combinedStats.StartTime = s.StartTime
				}
				if s.EndTime.After(combinedStats.EndTime) {
					combinedStats.EndTime = s.EndTime
				}
			}
		}
		if totalSamples > 0 {
			combinedStats.Average = totalUsage / float64(totalSamples)
			combinedStats.Samples = totalSamples
		}
		if tempSamples > 0 {
			combinedStats.TemperatureAvg = totalTemp / float64(tempSamples)
		}
		if clockSamples > 0 {
			combinedStats.ClockSpeedAvg = totalClock / float64(clockSamples)
		}
		if len(allStats) > 0 {
			combinedStats.EnergyConsumption = combinedStats.EnergyConsumption / float64(len(allStats))
			combinedStats.Duration = combinedStats.EndTime.Sub(combinedStats.StartTime)
		}

		displaySummary(allOutput.String(), combinedStats)
	}
}

func handleDiskMenu() {
	handleDiskBenchmarkMenu()
}

func handleDiskBenchmarkMenu() {
	fmt.Println("\n=== Benchmark de Disco Basado en Archivos ===")
	fmt.Println("Este benchmark fuerza el disco físico mediante escritura y lectura continua")
	fmt.Println()

	// Obtener espacio en disco antes del test
	diskBefore, errBefore := getDiskFreeSpace()
	if errBefore == nil {
		printInfo(fmt.Sprintf("Espacio libre en disco antes: %s", formatBytes(diskBefore)))
	}

	// Crear directorio temporal
	tempDir := filepath.Join(os.TempDir(), "disk_benchmark")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		printError(fmt.Sprintf("Error creando directorio temporal: %v", err))
		return
	}

	// Solicitar tamaño del test
	fmt.Print("Tamaño total de archivos en GiB (1.0): ")
	var sizeInput string
	fmt.Scanln(&sizeInput)
	aggregateSizeGiB := 1.0
	if sizeInput != "" {
		if val, err := strconv.ParseFloat(sizeInput, 64); err == nil && val > 0 {
			aggregateSizeGiB = val
		}
	}

	// Crear instancia del benchmark
	bm := disk.NewMark(tempDir, aggregateSizeGiB)

	// Asegurar eliminación completa del directorio al finalizar (incluso si hay errores)
	defer func() {
		printInfo("Limpiando archivos temporales...")

		// Primero limpiar archivos individuales
		if err := bm.CleanupTestFiles(); err != nil {
			printError(fmt.Sprintf("Error limpiando archivos: %v", err))
		}

		// Luego eliminar el directorio completo (fuerza bruta)
		if err := os.RemoveAll(tempDir); err != nil {
			printError(fmt.Sprintf("Error eliminando directorio temporal: %v", err))
			printError(fmt.Sprintf("Directorio que no se pudo eliminar: %s", tempDir))
		} else {
			printInfo("✓ Directorio temporal eliminado completamente")
		}

		// En Windows, el sistema puede tardar en actualizar el espacio libre después de eliminar archivos grandes
		// Esperar un momento para que el sistema actualice las estadísticas de disco
		printInfo("Esperando actualización del sistema de archivos...")
		time.Sleep(2 * time.Second)

		// Intentar múltiples veces para obtener el espacio libre actualizado
		var diskAfter uint64
		var errAfter error
		maxRetries := 5
		for i := 0; i < maxRetries; i++ {
			diskAfter, errAfter = getDiskFreeSpace()
			if errAfter == nil {
				// Esperar un poco más y verificar de nuevo para asegurar que el valor se haya actualizado
				if i < maxRetries-1 {
					time.Sleep(500 * time.Millisecond)
					newDiskAfter, newErr := getDiskFreeSpace()
					if newErr == nil && newDiskAfter == diskAfter {
						// El valor se estabilizó, usar este valor
						break
					}
					diskAfter = newDiskAfter
				}
			}
		}

		// Verificar espacio en disco después
		if errAfter == nil && errBefore == nil {
			spaceFreed := diskAfter - diskBefore
			if spaceFreed > 0 {
				printInfo(fmt.Sprintf("Espacio liberado: %s", formatBytes(spaceFreed)))
			}
			printInfo(fmt.Sprintf("Espacio libre en disco después: %s", formatBytes(diskAfter)))

			// Verificar que el espacio es igual o mayor que antes
			// Permitir una pequeña diferencia (menos de 100 MB) debido a posibles actualizaciones pendientes del sistema
			diff := diskBefore - diskAfter
			if diff < 100*1024*1024 { // Menos de 100 MB de diferencia
				if diskAfter >= diskBefore {
					printInfo("✓ Espacio en disco restaurado correctamente")
				} else {
					printInfo(fmt.Sprintf("✓ Espacio en disco restaurado (diferencia menor: %s, puede ser normal en Windows)",
						formatBytes(diff)))
				}
			} else {
				printError(fmt.Sprintf("⚠ Advertencia: El espacio en disco es menor que antes (diferencia: %s)",
					formatBytes(diff)))
				printInfo("   Nota: En Windows, el espacio puede tardar en actualizarse. Verifica manualmente si es necesario.")
			}
		}
	}()

	// Configurar directorio temporal
	if err := bm.SetTempDir(tempDir); err != nil {
		printError(fmt.Sprintf("Error configurando directorio: %v", err))
		return
	}

	// La limpieza se maneja en el defer principal del directorio

	// Generar bloque aleatorio de 64 KB
	printInfo("Generando datos aleatorios...")
	if err := bm.CreateRandomBlock(); err != nil {
		printError(fmt.Sprintf("Error generando bloque aleatorio: %v", err))
		return
	}

	// Iniciar limpieza continua de cache (en goroutine)
	bm.ClearBufferCacheEveryThreeSeconds()
	defer bm.StopCacheClear() // Detener al finalizar

	// Ejecutar ciclo completo de tests
	printInfo("Ejecutando test de escritura secuencial...")
	if err := bm.RunSequentialWriteTest(); err != nil {
		printError(fmt.Sprintf("Error en test de escritura: %v", err))
		return
	}

	printInfo("Ejecutando test de lectura secuencial...")
	if err := bm.RunSequentialReadTest(); err != nil {
		printError(fmt.Sprintf("Error en test de lectura: %v", err))
		return
	}

	printInfo("Ejecutando test de IOPS...")
	if err := bm.RunIOPSTest(); err != nil {
		printError(fmt.Sprintf("Error en test de IOPS: %v", err))
		return
	}

	printInfo("Ejecutando test de eliminación...")
	if err := bm.RunDeleteTest(); err != nil {
		printError(fmt.Sprintf("Error en test de eliminación: %v", err))
		// No retornar, continuar para mostrar resultados
	}

	// Mostrar resultados
	result := bm.GetLastResult()
	if result != nil {
		disk.DisplayDiskBenchmarkResult(result, formatBytes, colorBold, colorReset, colorGreen, colorCyan, colorYellow, colorRed, printHeader, printError)
	}

	// La limpieza se hace automáticamente con defer, pero también la hacemos aquí explícitamente
	// para tener feedback inmediato
	if err := bm.CleanupTestFiles(); err != nil {
		printError(fmt.Sprintf("Advertencia: error limpiando archivos: %v", err))
	} else {
		// Verificar que se eliminaron
		if count, err := bm.VerifyCleanup(); err == nil && count == 0 {
			printInfo("✓ Archivos de test eliminados correctamente")
		}
	}
}

// getDiskFreeSpace obtiene el espacio libre en disco del directorio temporal
func getDiskFreeSpace() (uint64, error) {
	tempDir := os.TempDir()
	usage, err := gopsutildisk.Usage(tempDir)
	if err != nil {
		return 0, err
	}
	return usage.Free, nil
}

func main() {
	fmt.Print("\033[2J\033[H")
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

	for {
		fmt.Println("\n" + strings.Repeat("-", 40))
		fmt.Println("1. Benchmarks CPU")
		fmt.Println("2. Test Disco")
		fmt.Println("3. Test RAM")
		fmt.Println("4. Info Sistema")
		fmt.Println("5. Baseline")
		fmt.Println("6. Salir")
		fmt.Print("\nOpción: ")

		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "1":
			handleCPUMenu()
		case "2":
			handleDiskMenu()
		case "3":
			handleRAMStressMenu()
		case "4":
			displaySystemInfo()
		case "5":
			displayBaseline()
		case "6":
			fmt.Println("\nSaliendo...")
			os.Exit(0)
		default:
			printError("Opción inválida")
		}
	}
}
