package ram

import (
	"fmt"
	"time"

	"v2/internal/baseline"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type IsolationReport struct {
	IsIsolated          bool
	IsolationScore      float64
	BaselineBefore      *baseline.BaselineResult
	Interferences       []Interference
	Warnings            []string
	Recommendations     []string
	CPUInterference     CPUInterferenceInfo
	SwapInterference    SwapInterferenceInfo
	ProcessInterference ProcessInterferenceInfo
	DiskInterference    DiskInterferenceInfo
	Optimizations       *OptimizationResult
}

type Interference struct {
	Type        string
	Severity    string
	Description string
	Timestamp   time.Time
	Value       float64
}

type CPUInterferenceInfo struct {
	BaselineCPUIdle      float64
	MinCPUIdleDuring     float64
	AvgCPUIdleDuring     float64
	MaxCPUUsage          float64
	InterferingProcesses []ProcessCPUInfo
}

type ProcessCPUInfo struct {
	PID        int32
	Name       string
	CPUPercent float64
	MemoryMB   float64
}

type SwapInterferenceInfo struct {
	BaselineSwapUsed  uint64
	MaxSwapUsedDuring uint64
	SwapUsedDuring    bool
	SwapUsagePercent  float64
}

type ProcessInterferenceInfo struct {
	BaselineHighMemoryProcesses int
	NewHighMemoryProcesses      int
	MemoryConsumedByOthers      uint64
	MemoryConsumedPercent       float64
}

type DiskInterferenceInfo struct {
	BaselineDiskReadMBs  float64
	BaselineDiskWriteMBs float64
	MaxDiskReadMBs       float64
	MaxDiskWriteMBs      float64
	HighDiskActivity     bool
}

func ValidateRAMIsolation(baselineResult *baseline.BaselineResult, testDuration time.Duration) (*IsolationReport, error) {
	report := &IsolationReport{
		IsolationScore:  100.0,
		BaselineBefore:  baselineResult,
		Interferences:   []Interference{},
		Warnings:        []string{},
		Recommendations: []string{},
	}

	if baselineResult == nil {
		return nil, fmt.Errorf("baseline result es requerido para validar aislamiento")
	}

	initialValidation := validateInitialState(baselineResult, report)
	if !initialValidation {
		report.Warnings = append(report.Warnings,
			"El estado inicial del sistema no es ideal para una prueba aislada de RAM")
	}

	monitorDone := make(chan struct{})
	monitorResults := make(chan IsolationMonitorResult, 100)

	go monitorSystemDuringTest(testDuration, monitorDone, monitorResults)

	time.Sleep(testDuration + 500*time.Millisecond)
	close(monitorDone)

	var monitorData []IsolationMonitorResult
	for {
		select {
		case result := <-monitorResults:
			monitorData = append(monitorData, result)
		default:
			goto done
		}
	}
done:

	analyzeInterferences(report, baselineResult, monitorData)

	calculateIsolationScore(report)

	report.IsIsolated = report.IsolationScore >= 80.0

	generateIsolationRecommendations(report)

	return report, nil
}

type IsolationMonitorResult struct {
	Timestamp    time.Time
	CPUIdle      float64
	SwapUsed     uint64
	HighMemProcs []ProcessCPUInfo
	DiskReadMBs  float64
	DiskWriteMBs float64
}

func validateInitialState(baseline *baseline.BaselineResult, report *IsolationReport) bool {
	isValid := true

	if baseline.Baseline.CPUIdlePercent < 50 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("CPU idle bajo (%.2f%%). Se recomienda >50%% para pruebas aisladas",
				baseline.Baseline.CPUIdlePercent))
		isValid = false
	}

	if len(baseline.Environment.HighCPUProcesses) > 5 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de CPU. Esto puede interferir con la prueba",
				len(baseline.Environment.HighCPUProcesses)))
		isValid = false
	}

	if len(baseline.Environment.HighMemoryProcesses) > 10 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de memoria. Esto puede interferir con la prueba",
				len(baseline.Environment.HighMemoryProcesses)))
		isValid = false
	}

	swap, err := mem.SwapMemory()
	if err == nil && swap.Used > 0 {
		swapPercent := float64(swap.Used) / float64(swap.Total) * 100
		if swapPercent > 5 {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("Swap ya está en uso (%.2f%%). Para pruebas aisladas, el swap debe estar libre",
					swapPercent))
			isValid = false
		}
	}

	return isValid
}

func monitorSystemDuringTest(duration time.Duration, done chan struct{}, results chan IsolationMonitorResult) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			result := IsolationMonitorResult{
				Timestamp: time.Now(),
			}

			cpuPercentages, err := cpu.Percent(200*time.Millisecond, false)
			if err == nil && len(cpuPercentages) > 0 {
				result.CPUIdle = 100.0 - cpuPercentages[0]
			}

			swap, err := mem.SwapMemory()
			if err == nil {
				result.SwapUsed = swap.Used
			}

			processes, err := process.Processes()
			if err == nil {
				for _, proc := range processes {
					if len(result.HighMemProcs) >= 10 {
						break
					}
					memInfo, _ := proc.MemoryInfo()
					if memInfo != nil && memInfo.RSS > 100*1024*1024 {
						name, _ := proc.Name()
						cpuPercent, _ := proc.CPUPercent()
						result.HighMemProcs = append(result.HighMemProcs, ProcessCPUInfo{
							PID:        proc.Pid,
							Name:       name,
							CPUPercent: cpuPercent,
							MemoryMB:   float64(memInfo.RSS) / (1024 * 1024),
						})
					}
				}
			}

			result.DiskReadMBs = 0
			result.DiskWriteMBs = 0

			select {
			case results <- result:
			default:
			}
		}
	}
}

func analyzeInterferences(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	if len(monitorData) == 0 {
		return
	}

	analyzeCPUInterference(report, baseline, monitorData)

	analyzeSwapInterference(report, baseline, monitorData)

	analyzeProcessInterference(report, baseline, monitorData)

	analyzeDiskInterference(report, baseline, monitorData)
}

func analyzeCPUInterference(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	baselineCPUIdle := baseline.Baseline.CPUIdlePercent
	report.CPUInterference.BaselineCPUIdle = baselineCPUIdle

	var minIdle float64 = 100.0
	var maxUsage float64 = 0.0
	var sumIdle float64 = 0.0
	var interferingProcs []ProcessCPUInfo

	for _, data := range monitorData {
		if data.CPUIdle < minIdle {
			minIdle = data.CPUIdle
		}
		if data.CPUIdle < minIdle {
			maxUsage = 100.0 - data.CPUIdle
		}
		sumIdle += data.CPUIdle

		for _, proc := range data.HighMemProcs {
			if proc.CPUPercent > 10 {
				interferingProcs = append(interferingProcs, proc)
			}
		}
	}

	report.CPUInterference.MinCPUIdleDuring = minIdle
	report.CPUInterference.AvgCPUIdleDuring = sumIdle / float64(len(monitorData))
	report.CPUInterference.MaxCPUUsage = maxUsage
	report.CPUInterference.InterferingProcesses = interferingProcs

	cpuIdleDrop := baselineCPUIdle - report.CPUInterference.AvgCPUIdleDuring
	if cpuIdleDrop > 20 {
		severity := "medium"
		if cpuIdleDrop > 40 {
			severity = "high"
		}
		report.Interferences = append(report.Interferences, Interference{
			Type:     "cpu",
			Severity: severity,
			Description: fmt.Sprintf("CPU idle cayó %.2f%% durante la prueba (de %.2f%% a %.2f%%)",
				cpuIdleDrop, baselineCPUIdle, report.CPUInterference.AvgCPUIdleDuring),
			Timestamp: time.Now(),
			Value:     cpuIdleDrop,
		})
	}

	if len(interferingProcs) > 0 {
		report.Interferences = append(report.Interferences, Interference{
			Type:        "cpu",
			Severity:    "medium",
			Description: fmt.Sprintf("%d procesos externos usando CPU durante la prueba", len(interferingProcs)),
			Timestamp:   time.Now(),
			Value:       float64(len(interferingProcs)),
		})
	}
}

func analyzeSwapInterference(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	swap, err := mem.SwapMemory()
	if err != nil {
		return
	}

	baselineSwap := swap.Used
	report.SwapInterference.BaselineSwapUsed = baselineSwap

	var maxSwap uint64 = baselineSwap
	swapUsed := false

	for _, data := range monitorData {
		if data.SwapUsed > maxSwap {
			maxSwap = data.SwapUsed
		}
		if data.SwapUsed > baselineSwap {
			swapUsed = true
		}
	}

	report.SwapInterference.MaxSwapUsedDuring = maxSwap
	report.SwapInterference.SwapUsedDuring = swapUsed

	if swap.Total > 0 {
		report.SwapInterference.SwapUsagePercent = float64(maxSwap) / float64(swap.Total) * 100
	}

	if swapUsed {
		swapIncrease := maxSwap - baselineSwap
		report.Interferences = append(report.Interferences, Interference{
			Type:     "swap",
			Severity: "critical",
			Description: fmt.Sprintf("Se detectó uso de swap durante la prueba (+%s). La prueba NO está aislada a RAM física",
				formatBytes(swapIncrease)),
			Timestamp: time.Now(),
			Value:     float64(swapIncrease),
		})
		report.Warnings = append(report.Warnings,
			"CRÍTICO: Se usó swap durante la prueba. La prueba NO está aislada únicamente a RAM física.")
	}
}

func analyzeProcessInterference(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	baselineHighMem := len(baseline.Environment.HighMemoryProcesses)
	report.ProcessInterference.BaselineHighMemoryProcesses = baselineHighMem

	procMap := make(map[int32]bool)
	for _, data := range monitorData {
		for _, proc := range data.HighMemProcs {
			procMap[proc.PID] = true
		}
	}

	newProcs := len(procMap) - baselineHighMem
	if newProcs < 0 {
		newProcs = 0
	}

	report.ProcessInterference.NewHighMemoryProcesses = newProcs

	totalOtherMemory := uint64(0)
	for _, data := range monitorData {
		for _, proc := range data.HighMemProcs {
			totalOtherMemory += uint64(proc.MemoryMB * 1024 * 1024)
		}
	}
	if len(monitorData) > 0 {
		totalOtherMemory = totalOtherMemory / uint64(len(monitorData))
	}

	report.ProcessInterference.MemoryConsumedByOthers = totalOtherMemory

	if baseline.Hardware.RAMTotal > 0 {
		report.ProcessInterference.MemoryConsumedPercent = float64(totalOtherMemory) / float64(baseline.Hardware.RAMTotal) * 100
	}

	if newProcs > 5 || report.ProcessInterference.MemoryConsumedPercent > 10 {
		severity := "medium"
		if newProcs > 10 || report.ProcessInterference.MemoryConsumedPercent > 20 {
			severity = "high"
		}
		report.Interferences = append(report.Interferences, Interference{
			Type:     "process",
			Severity: severity,
			Description: fmt.Sprintf("%d procesos externos consumiendo memoria (%.2f%% de RAM total)",
				newProcs, report.ProcessInterference.MemoryConsumedPercent),
			Timestamp: time.Now(),
			Value:     float64(newProcs),
		})
	}
}

func analyzeDiskInterference(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	report.DiskInterference.BaselineDiskReadMBs = 0
	report.DiskInterference.BaselineDiskWriteMBs = 0
	report.DiskInterference.MaxDiskReadMBs = 0
	report.DiskInterference.MaxDiskWriteMBs = 0
	report.DiskInterference.HighDiskActivity = false

}

func calculateIsolationScore(report *IsolationReport) {
	score := 100.0

	for _, interference := range report.Interferences {
		switch interference.Severity {
		case "critical":
			score -= 30.0
		case "high":
			score -= 20.0
		case "medium":
			score -= 10.0
		case "low":
			score -= 5.0
		}
	}

	if report.SwapInterference.SwapUsedDuring {
		score -= 40.0
	}

	if report.CPUInterference.AvgCPUIdleDuring < 30 {
		score -= 15.0
	} else if report.CPUInterference.AvgCPUIdleDuring < 50 {
		score -= 8.0
	}

	if len(report.CPUInterference.InterferingProcesses) > 5 {
		score -= 10.0
	}

	if report.ProcessInterference.MemoryConsumedPercent > 20 {
		score -= 15.0
	} else if report.ProcessInterference.MemoryConsumedPercent > 10 {
		score -= 8.0
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	report.IsolationScore = score
}

func generateIsolationRecommendations(report *IsolationReport) {
	if report.SwapInterference.SwapUsedDuring {
		report.Recommendations = append(report.Recommendations,
			"CRÍTICO: Desactivar swap o reducir el tamaño de la prueba para evitar uso de swap")
		report.Recommendations = append(report.Recommendations,
			"La prueba NO está aislada a RAM física si se usa swap")
	}

	if report.CPUInterference.AvgCPUIdleDuring < 50 {
		report.Recommendations = append(report.Recommendations,
			"Cerrar aplicaciones que consumen CPU antes de ejecutar la prueba")
		report.Recommendations = append(report.Recommendations,
			"Considerar ejecutar la prueba en un sistema más idle")
	}

	if len(report.CPUInterference.InterferingProcesses) > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Cerrar %d procesos que están interfiriendo con la prueba",
				len(report.CPUInterference.InterferingProcesses)))
	}

	if report.ProcessInterference.MemoryConsumedPercent > 10 {
		report.Recommendations = append(report.Recommendations,
			"Cerrar aplicaciones que consumen mucha memoria antes de ejecutar la prueba")
	}

	if report.IsolationScore < 80 {
		report.Recommendations = append(report.Recommendations,
			"Para una prueba completamente aislada, ejecutar en un sistema limpio con mínimo de procesos")
		report.Recommendations = append(report.Recommendations,
			"Considerar ejecutar la prueba en modo seguro o con servicios deshabilitados")
	}

	if len(report.Interferences) == 0 && report.IsolationScore >= 80 {
		report.Recommendations = append(report.Recommendations,
			" La prueba está bien aislada. No se detectaron interferencias significativas.")
	}
}

func FormatIsolationReport(report *IsolationReport) string {
	var output string

	output += "=== REPORTE DE AISLAMIENTO DE RAM ===\n\n"

	status := " NO AISLADO"
	if report.IsIsolated {
		status = " AISLADO"
	}
	output += fmt.Sprintf("Estado: %s\n", status)
	output += fmt.Sprintf("Puntuación de Aislamiento: %.2f/100\n\n", report.IsolationScore)

	if len(report.Interferences) > 0 {
		output += "--- INTERFERENCIAS DETECTADAS ---\n"
		for i, interference := range report.Interferences {
			output += fmt.Sprintf("%d. [%s] %s - %s\n",
				i+1, interference.Severity, interference.Type, interference.Description)
		}
		output += "\n"
	} else {
		output += " No se detectaron interferencias significativas\n\n"
	}

	output += "--- INTERFERENCIA DE CPU ---\n"
	output += fmt.Sprintf("CPU Idle Baseline: %.2f%%\n", report.CPUInterference.BaselineCPUIdle)
	output += fmt.Sprintf("CPU Idle Mínimo Durante Prueba: %.2f%%\n", report.CPUInterference.MinCPUIdleDuring)
	output += fmt.Sprintf("CPU Idle Promedio Durante Prueba: %.2f%%\n", report.CPUInterference.AvgCPUIdleDuring)
	output += fmt.Sprintf("CPU Uso Máximo Durante Prueba: %.2f%%\n", report.CPUInterference.MaxCPUUsage)
	if len(report.CPUInterference.InterferingProcesses) > 0 {
		output += fmt.Sprintf("Procesos Interfiriendo: %d\n", len(report.CPUInterference.InterferingProcesses))
		for _, proc := range report.CPUInterference.InterferingProcesses[:min(5, len(report.CPUInterference.InterferingProcesses))] {
			output += fmt.Sprintf("  - %s (PID: %d, CPU: %.2f%%, Mem: %.2f MB)\n",
				proc.Name, proc.PID, proc.CPUPercent, proc.MemoryMB)
		}
	}
	output += "\n"

	output += "--- INTERFERENCIA DE SWAP ---\n"
	output += fmt.Sprintf("Swap Usado Baseline: %s\n", formatBytes(report.SwapInterference.BaselineSwapUsed))
	output += fmt.Sprintf("Swap Máximo Durante Prueba: %s\n", formatBytes(report.SwapInterference.MaxSwapUsedDuring))
	if report.SwapInterference.SwapUsedDuring {
		output += "  CRÍTICO: Se usó swap durante la prueba\n"
		output += "   La prueba NO está aislada únicamente a RAM física\n"
	} else {
		output += " No se usó swap durante la prueba\n"
	}
	output += "\n"

	output += "--- INTERFERENCIA DE PROCESOS ---\n"
	output += fmt.Sprintf("Procesos con Alta Memoria (Baseline): %d\n", report.ProcessInterference.BaselineHighMemoryProcesses)
	output += fmt.Sprintf("Nuevos Procesos con Alta Memoria: %d\n", report.ProcessInterference.NewHighMemoryProcesses)
	output += fmt.Sprintf("Memoria Consumida por Otros: %s (%.2f%%)\n",
		formatBytes(report.ProcessInterference.MemoryConsumedByOthers),
		report.ProcessInterference.MemoryConsumedPercent)
	output += "\n"

	if len(report.Warnings) > 0 {
		output += "--- ADVERTENCIAS ---\n"
		for i, warning := range report.Warnings {
			output += fmt.Sprintf("%d. %s\n", i+1, warning)
		}
		output += "\n"
	}

	if len(report.Recommendations) > 0 {
		output += "--- RECOMENDACIONES ---\n"
		for i, rec := range report.Recommendations {
			output += fmt.Sprintf("%d. %s\n", i+1, rec)
		}
		output += "\n"
	}

	return output
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ValidateIsolationBeforeTest valida el estado del sistema antes de ejecutar una prueba
func ValidateIsolationBeforeTest(baselineResult *baseline.BaselineResult) (bool, []string) {
	if baselineResult == nil {
		return false, []string{"Baseline result es requerido para validar aislamiento"}
	}

	var warnings []string
	isValid := true

	// Verificar CPU idle
	if baselineResult.Baseline.CPUIdlePercent < 50 {
		warnings = append(warnings,
			fmt.Sprintf("CPU idle bajo (%.2f%%). Se recomienda >50%% para pruebas aisladas",
				baselineResult.Baseline.CPUIdlePercent))
		isValid = false
	}

	// Verificar procesos de alto consumo
	if len(baselineResult.Environment.HighCPUProcesses) > 5 {
		warnings = append(warnings,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de CPU. Cerrarlos antes de la prueba",
				len(baselineResult.Environment.HighCPUProcesses)))
		isValid = false
	}

	if len(baselineResult.Environment.HighMemoryProcesses) > 10 {
		warnings = append(warnings,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de memoria. Cerrarlos antes de la prueba",
				len(baselineResult.Environment.HighMemoryProcesses)))
		isValid = false
	}

	// Verificar swap
	swap, err := mem.SwapMemory()
	if err == nil && swap.Used > 0 {
		swapPercent := float64(swap.Used) / float64(swap.Total) * 100
		if swapPercent > 5 {
			warnings = append(warnings,
				fmt.Sprintf("Swap ya está en uso (%.2f%%). Para pruebas aisladas, el swap debe estar libre",
					swapPercent))
			isValid = false
		}
	}

	return isValid, warnings
}
