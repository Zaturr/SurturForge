package ram

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"v2/internal/baseline"
)

// IsolationReport contiene el reporte de aislamiento de la prueba de RAM
type IsolationReport struct {
	IsIsolated          bool                    // Si la prueba está realmente aislada
	IsolationScore      float64                 // Puntuación de aislamiento (0-100)
	BaselineBefore      *baseline.BaselineResult // Baseline antes de la prueba
	Interferences       []Interference          // Interferencias detectadas
	Warnings            []string                // Advertencias sobre el aislamiento
	Recommendations     []string                // Recomendaciones para mejorar el aislamiento
	CPUInterference     CPUInterferenceInfo     // Información sobre interferencia de CPU
	SwapInterference    SwapInterferenceInfo    // Información sobre interferencia de Swap
	ProcessInterference ProcessInterferenceInfo // Información sobre interferencia de procesos
	DiskInterference    DiskInterferenceInfo    // Información sobre interferencia de disco
}

// Interference representa una interferencia detectada
type Interference struct {
	Type        string    // "cpu", "swap", "process", "disk"
	Severity    string    // "low", "medium", "high", "critical"
	Description string    // Descripción de la interferencia
	Timestamp   time.Time // Cuándo se detectó
	Value       float64   // Valor numérico de la interferencia
}

// CPUInterferenceInfo información sobre interferencia de CPU
type CPUInterferenceInfo struct {
	BaselineCPUIdle    float64   // CPU idle en baseline
	MinCPUIdleDuring   float64   // CPU idle mínimo durante la prueba
	AvgCPUIdleDuring   float64   // CPU idle promedio durante la prueba
	MaxCPUUsage        float64   // Máximo uso de CPU durante la prueba
	InterferingProcesses []ProcessCPUInfo // Procesos que interfieren
}

// ProcessCPUInfo información de un proceso que interfiere
type ProcessCPUInfo struct {
	PID        int32
	Name       string
	CPUPercent float64
	MemoryMB   float64
}

// SwapInterferenceInfo información sobre interferencia de Swap
type SwapInterferenceInfo struct {
	BaselineSwapUsed    uint64  // Swap usado en baseline
	MaxSwapUsedDuring   uint64  // Swap máximo usado durante la prueba
	SwapUsedDuring      bool    // Si se usó swap durante la prueba
	SwapUsagePercent    float64 // Porcentaje de swap usado
}

// ProcessInterferenceInfo información sobre interferencia de otros procesos
type ProcessInterferenceInfo struct {
	BaselineHighMemoryProcesses int     // Procesos con alta memoria en baseline
	NewHighMemoryProcesses       int     // Nuevos procesos con alta memoria durante la prueba
	MemoryConsumedByOthers       uint64  // Memoria consumida por otros procesos (bytes)
	MemoryConsumedPercent        float64 // Porcentaje de memoria consumida por otros
}

// DiskInterferenceInfo información sobre interferencia de disco
type DiskInterferenceInfo struct {
	BaselineDiskReadMBs  float64 // Lectura de disco en baseline (MB/s)
	BaselineDiskWriteMBs float64 // Escritura de disco en baseline (MB/s)
	MaxDiskReadMBs       float64 // Máxima lectura durante la prueba
	MaxDiskWriteMBs      float64 // Máxima escritura durante la prueba
	HighDiskActivity     bool    // Si hubo alta actividad de disco
}

// ValidateRAMIsolation valida que la prueba de RAM esté aislada de otros componentes
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

	// Validar estado inicial antes de la prueba
	initialValidation := validateInitialState(baselineResult, report)
	if !initialValidation {
		report.Warnings = append(report.Warnings,
			"El estado inicial del sistema no es ideal para una prueba aislada de RAM")
	}

	// Monitorear durante la prueba
	monitorDone := make(chan struct{})
	monitorResults := make(chan IsolationMonitorResult, 100)
	
	go monitorSystemDuringTest(testDuration, monitorDone, monitorResults)

	// Esperar a que termine el monitoreo
	time.Sleep(testDuration + 500*time.Millisecond)
	close(monitorDone)

	// Recopilar resultados del monitoreo
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

	// Analizar interferencias
	analyzeInterferences(report, baselineResult, monitorData)

	// Calcular puntuación de aislamiento
	calculateIsolationScore(report)

	// Determinar si está aislado (score > 80)
	report.IsIsolated = report.IsolationScore >= 80.0

	// Generar recomendaciones
	generateIsolationRecommendations(report)

	return report, nil
}

// IsolationMonitorResult resultado de una muestra de monitoreo
type IsolationMonitorResult struct {
	Timestamp      time.Time
	CPUIdle        float64
	SwapUsed       uint64
	HighMemProcs   []ProcessCPUInfo
	DiskReadMBs    float64
	DiskWriteMBs   float64
}

// validateInitialState valida el estado inicial antes de la prueba
func validateInitialState(baseline *baseline.BaselineResult, report *IsolationReport) bool {
	isValid := true

	// Verificar CPU idle (debe ser > 50%)
	if baseline.Baseline.CPUIdlePercent < 50 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("CPU idle bajo (%.2f%%). Se recomienda >50%% para pruebas aisladas",
				baseline.Baseline.CPUIdlePercent))
		isValid = false
	}

	// Verificar procesos de alto consumo de CPU
	if len(baseline.Environment.HighCPUProcesses) > 5 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de CPU. Esto puede interferir con la prueba",
				len(baseline.Environment.HighCPUProcesses)))
		isValid = false
	}

	// Verificar procesos de alto consumo de memoria
	if len(baseline.Environment.HighMemoryProcesses) > 10 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de memoria. Esto puede interferir con la prueba",
				len(baseline.Environment.HighMemoryProcesses)))
		isValid = false
	}

	// Verificar swap inicial (debe ser 0 o muy bajo)
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

// monitorSystemDuringTest monitorea el sistema durante la prueba
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

			// Monitorear CPU
			cpuPercentages, err := cpu.Percent(200*time.Millisecond, false)
			if err == nil && len(cpuPercentages) > 0 {
				result.CPUIdle = 100.0 - cpuPercentages[0]
			}

			// Monitorear Swap
			swap, err := mem.SwapMemory()
			if err == nil {
				result.SwapUsed = swap.Used
			}

			// Monitorear procesos con alto uso de memoria
			processes, err := process.Processes()
			if err == nil {
				for _, proc := range processes {
					if len(result.HighMemProcs) >= 10 {
						break
					}
					memInfo, _ := proc.MemoryInfo()
					if memInfo != nil && memInfo.RSS > 100*1024*1024 { // > 100MB
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

			// Monitorear I/O de disco (aproximado)
			// Nota: gopsutil no tiene una forma directa de obtener I/O instantáneo
			// Se puede usar un promedio o dejar en 0 si no está disponible
			result.DiskReadMBs = 0
			result.DiskWriteMBs = 0

			select {
			case results <- result:
			default:
				// Canal lleno, omitir esta muestra
			}
		}
	}
}

// analyzeInterferences analiza las interferencias detectadas
func analyzeInterferences(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	if len(monitorData) == 0 {
		return
	}

	// Analizar CPU
	analyzeCPUInterference(report, baseline, monitorData)

	// Analizar Swap
	analyzeSwapInterference(report, baseline, monitorData)

	// Analizar Procesos
	analyzeProcessInterference(report, baseline, monitorData)

	// Analizar Disco
	analyzeDiskInterference(report, baseline, monitorData)
}

// analyzeCPUInterference analiza interferencias de CPU
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

		// Identificar procesos que interfieren
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

	// Detectar interferencias significativas
	cpuIdleDrop := baselineCPUIdle - report.CPUInterference.AvgCPUIdleDuring
	if cpuIdleDrop > 20 {
		severity := "medium"
		if cpuIdleDrop > 40 {
			severity = "high"
		}
		report.Interferences = append(report.Interferences, Interference{
			Type:        "cpu",
			Severity:    severity,
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

// analyzeSwapInterference analiza interferencias de Swap
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

	// Si se usó swap, es una interferencia crítica
	if swapUsed {
		swapIncrease := maxSwap - baselineSwap
		report.Interferences = append(report.Interferences, Interference{
			Type:        "swap",
			Severity:    "critical",
			Description: fmt.Sprintf("Se detectó uso de swap durante la prueba (+%s). La prueba NO está aislada a RAM física",
				formatBytes(swapIncrease)),
			Timestamp: time.Now(),
			Value:     float64(swapIncrease),
		})
		report.Warnings = append(report.Warnings,
			"CRÍTICO: Se usó swap durante la prueba. La prueba NO está aislada únicamente a RAM física.")
	}
}

// analyzeProcessInterference analiza interferencias de otros procesos
func analyzeProcessInterference(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	baselineHighMem := len(baseline.Environment.HighMemoryProcesses)
	report.ProcessInterference.BaselineHighMemoryProcesses = baselineHighMem

	// Contar procesos únicos con alta memoria durante la prueba
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

	// Calcular memoria consumida por otros procesos (aproximado)
	// Esto es una estimación basada en los procesos detectados
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

	// Si hay muchos procesos nuevos o consumen mucha memoria, es una interferencia
	if newProcs > 5 || report.ProcessInterference.MemoryConsumedPercent > 10 {
		severity := "medium"
		if newProcs > 10 || report.ProcessInterference.MemoryConsumedPercent > 20 {
			severity = "high"
		}
		report.Interferences = append(report.Interferences, Interference{
			Type:        "process",
			Severity:    severity,
			Description: fmt.Sprintf("%d procesos externos consumiendo memoria (%.2f%% de RAM total)",
				newProcs, report.ProcessInterference.MemoryConsumedPercent),
			Timestamp: time.Now(),
			Value:     float64(newProcs),
		})
	}
}

// analyzeDiskInterference analiza interferencias de disco
func analyzeDiskInterference(report *IsolationReport, baseline *baseline.BaselineResult, monitorData []IsolationMonitorResult) {
	// Nota: gopsutil no proporciona I/O instantáneo fácilmente
	// Por ahora, dejamos esto como placeholder
	report.DiskInterference.BaselineDiskReadMBs = 0
	report.DiskInterference.BaselineDiskWriteMBs = 0
	report.DiskInterference.MaxDiskReadMBs = 0
	report.DiskInterference.MaxDiskWriteMBs = 0
	report.DiskInterference.HighDiskActivity = false

	// Si en el futuro se implementa monitoreo de I/O de disco, se puede analizar aquí
}

// calculateIsolationScore calcula la puntuación de aislamiento (0-100)
func calculateIsolationScore(report *IsolationReport) {
	score := 100.0

	// Penalizar por interferencias
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

	// Penalizar por uso de swap (crítico)
	if report.SwapInterference.SwapUsedDuring {
		score -= 40.0
	}

	// Penalizar por CPU idle bajo durante la prueba
	if report.CPUInterference.AvgCPUIdleDuring < 30 {
		score -= 15.0
	} else if report.CPUInterference.AvgCPUIdleDuring < 50 {
		score -= 8.0
	}

	// Penalizar por muchos procesos interfiriendo
	if len(report.CPUInterference.InterferingProcesses) > 5 {
		score -= 10.0
	}

	// Penalizar por memoria consumida por otros procesos
	if report.ProcessInterference.MemoryConsumedPercent > 20 {
		score -= 15.0
	} else if report.ProcessInterference.MemoryConsumedPercent > 10 {
		score -= 8.0
	}

	// Asegurar que el score esté en el rango 0-100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	report.IsolationScore = score
}

// generateIsolationRecommendations genera recomendaciones para mejorar el aislamiento
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
			"✓ La prueba está bien aislada. No se detectaron interferencias significativas.")
	}
}

// FormatIsolationReport formatea el reporte de aislamiento para impresión
func FormatIsolationReport(report *IsolationReport) string {
	var output string

	output += "=== REPORTE DE AISLAMIENTO DE RAM ===\n\n"

	// Estado de aislamiento
	status := "❌ NO AISLADO"
	if report.IsIsolated {
		status = "✓ AISLADO"
	}
	output += fmt.Sprintf("Estado: %s\n", status)
	output += fmt.Sprintf("Puntuación de Aislamiento: %.2f/100\n\n", report.IsolationScore)

	// Interferencias detectadas
	if len(report.Interferences) > 0 {
		output += "--- INTERFERENCIAS DETECTADAS ---\n"
		for i, interference := range report.Interferences {
			output += fmt.Sprintf("%d. [%s] %s - %s\n",
				i+1, interference.Severity, interference.Type, interference.Description)
		}
		output += "\n"
	} else {
		output += "✓ No se detectaron interferencias significativas\n\n"
	}

	// Información de CPU
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

	// Información de Swap
	output += "--- INTERFERENCIA DE SWAP ---\n"
	output += fmt.Sprintf("Swap Usado Baseline: %s\n", formatBytes(report.SwapInterference.BaselineSwapUsed))
	output += fmt.Sprintf("Swap Máximo Durante Prueba: %s\n", formatBytes(report.SwapInterference.MaxSwapUsedDuring))
	if report.SwapInterference.SwapUsedDuring {
		output += "⚠️  CRÍTICO: Se usó swap durante la prueba\n"
		output += "   La prueba NO está aislada únicamente a RAM física\n"
	} else {
		output += "✓ No se usó swap durante la prueba\n"
	}
	output += "\n"

	// Información de Procesos
	output += "--- INTERFERENCIA DE PROCESOS ---\n"
	output += fmt.Sprintf("Procesos con Alta Memoria (Baseline): %d\n", report.ProcessInterference.BaselineHighMemoryProcesses)
	output += fmt.Sprintf("Nuevos Procesos con Alta Memoria: %d\n", report.ProcessInterference.NewHighMemoryProcesses)
	output += fmt.Sprintf("Memoria Consumida por Otros: %s (%.2f%%)\n",
		formatBytes(report.ProcessInterference.MemoryConsumedByOthers),
		report.ProcessInterference.MemoryConsumedPercent)
	output += "\n"

	// Advertencias
	if len(report.Warnings) > 0 {
		output += "--- ADVERTENCIAS ---\n"
		for i, warning := range report.Warnings {
			output += fmt.Sprintf("%d. %s\n", i+1, warning)
		}
		output += "\n"
	}

	// Recomendaciones
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
// Retorna true si el sistema está en condiciones adecuadas para una prueba aislada
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

