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
	fmt.Printf("%sℹ %s%s\n", colorCyan, text, colorReset)
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

	if info, err := cpu.GetCPUInfo(); err == nil {
		fmt.Println(info)
	}

	if metrics, err := cpu.GetCPUMetrics(); err == nil {
		fmt.Printf("CPU: %.1f%% | Cores: %d/%d\n", metrics.UsagePercent, metrics.Cores, metrics.Threads)
	}

	if info, err := disk.GetDiskInfo(); err == nil {
		fmt.Println(info)
	}

	fmt.Printf("Go: %s | OS: %s | CPUs: %d\n", runtime.Version(), runtime.GOOS, runtime.NumCPU())
}

func displayBaseline() {
	printHeader("BASELINE DEL SISTEMA")

	if err := baseline.SetEnvironmentVariables(); err != nil {
		printError(fmt.Sprintf("Error variables: %v", err))
	}

	result, err := baseline.RunBaseline()
	if err != nil {
		printError(fmt.Sprintf("Error: %v", err))
		return
	}

	fmt.Printf("CPU Idle: %.1f%% | RAM: %s | Disco: %s\n",
		result.Baseline.CPUIdlePercent,
		baseline.FormatBytes(result.Baseline.MemoryAvailable),
		baseline.FormatBytes(result.Baseline.DiskFree))

	// Procesos de alto consumo
	if len(result.Environment.HighCPUProcesses) > 0 {
		sort.Slice(result.Environment.HighCPUProcesses, func(i, j int) bool {
			return result.Environment.HighCPUProcesses[i].CPUPercent > result.Environment.HighCPUProcesses[j].CPUPercent
		})
		fmt.Printf("\n%sProcesos CPU (>5%%):%s\n", colorYellow, colorReset)
		for i, p := range result.Environment.HighCPUProcesses {
			if i >= 10 {
				break
			}
			fmt.Printf("  %s (PID: %d) - %.1f%%\n", p.Name, p.PID, p.CPUPercent)
		}
	}

	if len(result.Environment.HighMemoryProcesses) > 0 {
		sort.Slice(result.Environment.HighMemoryProcesses, func(i, j int) bool {
			return result.Environment.HighMemoryProcesses[i].MemoryMB > result.Environment.HighMemoryProcesses[j].MemoryMB
		})
		fmt.Printf("\n%sProcesos Memoria (>100MB):%s\n", colorYellow, colorReset)
		for i, p := range result.Environment.HighMemoryProcesses {
			if i >= 10 {
				break
			}
			fmt.Printf("  %s (PID: %d) - %.1f MB\n", p.Name, p.PID, p.MemoryMB)
		}
	}
}

type CPUStats struct {
	Min, Max, Average float64
	Samples           int
}

func monitorCPU(done chan struct{}) *CPUStats {
	stats := &CPUStats{Min: 100.0}
	var total float64
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			if stats.Samples > 0 {
				stats.Average = total / float64(stats.Samples)
			}
			return stats
		case <-ticker.C:
			if m, err := cpu.GetCPUMetrics(); err == nil {
				usage := m.UsagePercent
				stats.Samples++
				total += usage
				if usage < stats.Min {
					stats.Min = usage
				}
				if usage > stats.Max {
					stats.Max = usage
				}
			}
		}
	}
}

func runBenchmark(pattern, desc string) (string, *CPUStats, error) {
	printInfo(desc)
	done := make(chan struct{})
	statsChan := make(chan *CPUStats, 1)

	go func() {
		statsChan <- monitorCPU(done)
	}()

	cmd := exec.Command("go", "test", "-bench", pattern, "-benchmem", "-benchtime=3s", "./internal/cpu")
	output, err := cmd.CombinedOutput()
	close(done)

	var stats *CPUStats
	select {
	case stats = <-statsChan:
	case <-time.After(2 * time.Second):
		stats = &CPUStats{}
	}

	if err != nil {
		return "", stats, fmt.Errorf("%v\n%s", err, string(output))
	}
	return string(output), stats, nil
}

type BenchmarkResult struct {
	Name       string
	Iterations string
	TimePerOp  string
	Allocs     string
	Bytes      string
}

func parseBenchmarkOutput(output string) map[string]*BenchmarkResult {
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

func formatTime(ns string) string {
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

func findBenchmark(benchmarks map[string]*BenchmarkResult, name string) *BenchmarkResult {
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

func displaySummary(output string, stats *CPUStats) {
	benchmarks := parseBenchmarkOutput(output)

	printHeader("RESULTADOS BENCHMARKS")

	// Mostrar benchmarks principales
	fmt.Printf("%s%-35s %12s %20s%s\n", colorBold, "Benchmark", "Iteraciones", "Tiempo/Op", colorReset)
	fmt.Println(strings.Repeat("-", 70))

	for name, result := range benchmarks {
		fmt.Printf("%-35s %12s %20s\n", name, result.Iterations, formatTime(result.TimePerOp))
	}

	// Comparación Single-Core vs Multi-Core
	singleSHA := findBenchmark(benchmarks, "BenchmarkSHA256SingleCore")
	multiSHA := findBenchmark(benchmarks, "BenchmarkSHA256MultiCore")
	if singleSHA != nil && multiSHA != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleSHA.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiSHA.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			improvement := float64(singleNs) / float64(multiNs)
			fmt.Printf("\n%sSHA-256:%s Single: %s | Multi: %s | %s%.2fx más rápido%s\n",
				colorCyan, colorReset,
				formatTime(singleSHA.TimePerOp),
				formatTime(multiSHA.TimePerOp),
				colorGreen, improvement, colorReset)
		}
	}

	singleAES := findBenchmark(benchmarks, "BenchmarkAES256SingleCore")
	multiAES := findBenchmark(benchmarks, "BenchmarkAES256MultiCore")
	if singleAES != nil && multiAES != nil {
		var singleNs, multiNs int64
		fmt.Sscanf(singleAES.TimePerOp, "%d", &singleNs)
		fmt.Sscanf(multiAES.TimePerOp, "%d", &multiNs)
		if singleNs > 0 && multiNs > 0 {
			improvement := float64(singleNs) / float64(multiNs)
			fmt.Printf("%sAES-256:%s Single: %s | Multi: %s | %s%.2fx más rápido%s\n",
				colorCyan, colorReset,
				formatTime(singleAES.TimePerOp),
				formatTime(multiAES.TimePerOp),
				colorGreen, improvement, colorReset)
		}
	}

	// Estadísticas de CPU durante el benchmark
	if stats != nil && stats.Samples > 0 {
		printSection("Estadísticas de CPU")
		fmt.Printf("Uso Mínimo: %s%.1f%%%s | Máximo: %s%.1f%%%s | Promedio: %s%.1f%%%s\n",
			colorGreen, stats.Min, colorReset,
			colorRed, stats.Max, colorReset,
			colorYellow, stats.Average, colorReset)
		fmt.Printf("Muestras: %d\n", stats.Samples)

		// Interpretación
		if stats.Average < 30 {
			fmt.Printf("%s CPU con bajo uso durante el benchmark%s\n", colorGreen, colorReset)
		} else if stats.Average < 70 {
			fmt.Printf("%s CPU con uso moderado durante el benchmark%s\n", colorYellow, colorReset)
		} else {
			fmt.Printf("%s CPU con alto uso durante el benchmark%s\n", colorRed, colorReset)
		}
	}

	// Explicación
	printSection("Explicación")
	fmt.Println("• Tiempo/Op: Tiempo promedio por operación (menor = mejor)")
	fmt.Println("• Iteraciones: Número de veces que se ejecutó el benchmark")
	fmt.Println("• Mejora Multi-Core: Factor de mejora usando múltiples cores")
	fmt.Println("• CPU: Uso del procesador durante la ejecución del benchmark")
}

func runDiskStressTest(config disk.DiskStressConfig) (*disk.DiskStressResult, error) {
	printInfo(fmt.Sprintf("Ejecutando test de disco (%v)...", config.TestDuration))
	return disk.RunDiskStress(config)
}

func displayDiskStressResult(result *disk.DiskStressResult) {
	printHeader("RESULTADOS DISCO")
	duration := result.EndTime.Sub(result.StartTime).Seconds()

	fmt.Printf("Escrituras: %d | Lecturas: %d | Errores: %d\n",
		result.TotalWrites, result.TotalReads, result.Errors)

	if duration > 0 {
		totalMB := float64(result.TotalBytesWritten+result.TotalBytesRead) / (1024 * 1024)
		fmt.Printf("Throughput: %.2f MB/s | IOPS: %.0f\n",
			totalMB/duration,
			float64(result.TotalWrites+result.TotalReads)/duration)
	}
}

func getDiskStressConfig() disk.DiskStressConfig {
	config := disk.DiskStressConfig{
		DBPath:             filepath.Join(os.TempDir(), "disk_stress_test.db"),
		WriteWorkers:       4,
		ReadWorkers:        4,
		DeleteWorkers:      2,
		OperationsPerCycle: 100,
		TestDuration:       30 * time.Second,
		DataSize:           1024,
	}

	fmt.Println("\nConfiguración (Enter = default):")
	if val := readInt("Workers Escritura (4): ", 4); val > 0 {
		config.WriteWorkers = val
	}
	if val := readInt("Workers Lectura (4): ", 4); val > 0 {
		config.ReadWorkers = val
	}
	if val := readInt("Duración segundos (30): ", 30); val > 0 {
		config.TestDuration = time.Duration(val) * time.Second
	}
	return config
}

func readInt(prompt string, def int) int {
	fmt.Print(prompt)
	var s string
	fmt.Scanln(&s)
	if s == "" {
		return def
	}
	if val, err := strconv.Atoi(s); err == nil && val > 0 {
		return val
	}
	return def
}

func runRAMStressTest(intensity string) {
	config := ram.GetIntensityConfig(intensity)
	result, err := ram.RunRAMStress(config)
	if err != nil {
		printError(fmt.Sprintf("Error: %v", err))
		return
	}
	displayRAMStressResult(result)
}

func displayRAMStressResult(result *ram.RAMStressResult) {
	printHeader("RESULTADOS RAM")
	duration := result.EndTime.Sub(result.StartTime)

	fmt.Printf("Operaciones: %d | Throughput: %.2f MB/s\n", result.Operations, result.ThroughputMBs)
	fmt.Printf("RAM: %.1f%% → %.1f%% | Degradación: %.1f%%\n",
		result.InitialMemory.UsagePercent,
		result.FinalMemory.UsagePercent,
		result.Degradation)

	if result.SwapUsed {
		printError("Swap usado durante el test")
	}
	fmt.Printf("Estabilidad: %.1f/100 | Duración: %v\n", result.StabilityScore, duration)
}

func runFullRAMEvaluation() {
	printHeader("EVALUACIÓN RAM")
	printInfo("Ejecutando...")

	baselineResult, _ := baseline.RunBaseline()
	baselinePtr := &baselineResult

	strategy := ram.GetRecommendedEvaluationStrategy()
	evaluation, err := ram.RunFullRAMEvaluation(strategy, baselinePtr)
	if err != nil {
		printError(fmt.Sprintf("Error: %v", err))
		return
	}

	printHeader("RESULTADOS")
	if evaluation.LightTestResult != nil {
		fmt.Printf("Ligero: %.2f MB/s | Deg: %.1f%%\n",
			evaluation.LightTestResult.ThroughputMBs,
			evaluation.LightTestResult.Degradation)
	}
	if evaluation.MediumTestResult != nil {
		fmt.Printf("Medio: %.2f MB/s | Deg: %.1f%%\n",
			evaluation.MediumTestResult.ThroughputMBs,
			evaluation.MediumTestResult.Degradation)
	}
	if evaluation.HeavyTestResult != nil {
		fmt.Printf("Pesado: %.2f MB/s | Deg: %.1f%%\n",
			evaluation.HeavyTestResult.ThroughputMBs,
			evaluation.HeavyTestResult.Degradation)
	}
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
		var allStats []*CPUStats

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
		combinedStats := &CPUStats{Min: 100.0}
		var totalUsage float64
		totalSamples := 0
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
			}
		}
		if totalSamples > 0 {
			combinedStats.Average = totalUsage / float64(totalSamples)
			combinedStats.Samples = totalSamples
		}

		displaySummary(allOutput.String(), combinedStats)
	}
}

func handleDiskMenu() {
	fmt.Println("\n1. Configuración por defecto")
	fmt.Println("2. Configuración personalizada")
	fmt.Print("Opción: ")

	var choice string
	fmt.Scanln(&choice)

	var config disk.DiskStressConfig
	if choice == "2" {
		config = getDiskStressConfig()
	} else {
		config = disk.DiskStressConfig{
			DBPath:             filepath.Join(os.TempDir(), "disk_stress_test.db"),
			WriteWorkers:       4,
			ReadWorkers:        4,
			DeleteWorkers:      2,
			OperationsPerCycle: 100,
			TestDuration:       30 * time.Second,
			DataSize:           1024,
		}
	}

	result, err := runDiskStressTest(config)
	if err != nil {
		printError(fmt.Sprintf("Error: %v", err))
		return
	}
	displayDiskStressResult(result)
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
