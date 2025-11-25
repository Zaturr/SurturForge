package ram

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"v2/internal/baseline"

	"github.com/shirou/gopsutil/v4/mem"
)

// RAMStressConfig configuración para el test de estrés de RAM
type RAMStressConfig struct {
	// Porcentaje de RAM a usar (0.1 = 10%, 0.5 = 50%, etc.)
	MemoryPercentage float64
	// Duración del test
	Duration time.Duration
	// Número de goroutines trabajando simultáneamente
	Workers int
	// Tamaño de bloque para operaciones (en bytes)
	BlockSize int
	// Tipo de operación: "read", "write", "mixed"
	OperationType string
	// Intensidad: "light", "medium", "heavy", "extreme"
	Intensity string
}

// RAMStressResult resultado del test de estrés
type RAMStressResult struct {
	Config          RAMStressConfig
	StartTime       time.Time
	EndTime         time.Time
	InitialMemory   RAMMetrics
	FinalMemory     RAMMetrics
	PeakMemory      RAMMetrics
	Operations      int64
	ThroughputMBs   float64
	Errors          int
	SwapUsed        bool
	Degradation     float64          // Degradación de rendimiento (%)
	StabilityScore  float64          // Puntuación de estabilidad (0-100)
	IsolationReport *IsolationReport // Reporte de aislamiento (si se validó)
}

// DefaultRAMStressConfig retorna una configuración por defecto segura
func DefaultRAMStressConfig() RAMStressConfig {
	availablePercent := 0.1 // Usar solo 10% por defecto para ser seguro

	return RAMStressConfig{
		MemoryPercentage: availablePercent,
		Duration:         30 * time.Second,
		Workers:          runtime.NumCPU(),
		BlockSize:        1024 * 1024, // 1MB
		OperationType:    "mixed",
		Intensity:        "medium",
	}
}

// GetIntensityConfig retorna configuración basada en la intensidad
func GetIntensityConfig(intensity string) RAMStressConfig {
	config := DefaultRAMStressConfig()
	config.Intensity = intensity

	switch intensity {
	case "light":
		config.MemoryPercentage = 0.05 // 5%
		config.Duration = 15 * time.Second
		config.Workers = runtime.NumCPU() / 2
		config.BlockSize = 512 * 1024 // 512KB
		config.OperationType = "read"

	case "medium":
		config.MemoryPercentage = 0.15 // 15%
		config.Duration = 30 * time.Second
		config.Workers = runtime.NumCPU()
		config.BlockSize = 1024 * 1024 // 1MB
		config.OperationType = "mixed"

	case "heavy":
		config.MemoryPercentage = 0.30 // 30%
		config.Duration = 60 * time.Second
		config.Workers = runtime.NumCPU() * 2
		config.BlockSize = 2 * 1024 * 1024 // 2MB
		config.OperationType = "mixed"

	case "extreme":
		config.MemoryPercentage = 0.50 // 50%
		config.Duration = 120 * time.Second
		config.Workers = runtime.NumCPU() * 4
		config.BlockSize = 4 * 1024 * 1024 // 4MB
		config.OperationType = "mixed"
	}

	return config
}

// RunRAMStress ejecuta un test de estrés de RAM
// Si baselineResult no es nil, valida el aislamiento de la prueba
func RunRAMStress(config RAMStressConfig, baselineResult ...*baseline.BaselineResult) (*RAMStressResult, error) {
	result := &RAMStressResult{
		Config:    config,
		StartTime: time.Now(),
	}

	// Obtener estado inicial
	initialMetrics, err := GetRAMMetrics()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo métricas iniciales: %w", err)
	}
	result.InitialMemory = initialMetrics

	// Validar aislamiento si se proporciona baseline
	var baselineForIsolation *baseline.BaselineResult
	if len(baselineResult) > 0 && baselineResult[0] != nil {
		baselineForIsolation = baselineResult[0]
	}

	// Iniciar validación de aislamiento en paralelo si hay baseline
	var isolationReportChan chan *IsolationReport
	if baselineForIsolation != nil {
		isolationReportChan = make(chan *IsolationReport, 1)
		go func() {
			report, err := ValidateRAMIsolation(baselineForIsolation, config.Duration)
			if err == nil {
				isolationReportChan <- report
			} else {
				isolationReportChan <- nil
			}
		}()
	}

	// Calcular cantidad de memoria a usar
	vmem, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo información de memoria: %w", err)
	}

	// Usar el porcentaje de memoria disponible, no total
	availableBytes := uint64(float64(vmem.Available) * config.MemoryPercentage)
	if availableBytes < uint64(config.BlockSize) {
		availableBytes = uint64(config.BlockSize) * 10 // Mínimo 10 bloques
	}

	// Crear buffers de memoria
	buffers := make([][]byte, config.Workers)
	for i := range buffers {
		buffers[i] = make([]byte, config.BlockSize)
	}

	// Canal para controlar workers
	done := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var operations int64
	var errors int64

	// Monitorear memoria durante el test
	peakMemory := initialMetrics
	monitorDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-monitorDone:
				return
			case <-ticker.C:
				current, err := GetRAMMetrics()
				if err == nil {
					mu.Lock()
					if current.UsedRAM > peakMemory.UsedRAM {
						peakMemory = current
					}
					mu.Unlock()
				}
			}
		}
	}()

	// Iniciar workers
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			buffer := buffers[workerID]
			localOps := int64(0)

			for {
				select {
				case <-done:
					mu.Lock()
					operations += localOps
					mu.Unlock()
					return
				default:
					// Realizar operaciones según el tipo
					switch config.OperationType {
					case "read":
						// Simular lectura: acceder a memoria
						for j := 0; j < len(buffer); j += 64 {
							_ = buffer[j]
						}
					case "write":
						// Simular escritura: escribir patrones
						for j := 0; j < len(buffer); j++ {
							buffer[j] = byte(j % 256)
						}
					case "mixed":
						// Mezcla de lectura y escritura
						if localOps%2 == 0 {
							for j := 0; j < len(buffer); j += 64 {
								_ = buffer[j]
							}
						} else {
							for j := 0; j < len(buffer); j++ {
								buffer[j] = byte((j + workerID) % 256)
							}
						}
					}
					localOps++
				}
			}
		}(i)
	}

	// Ejecutar por la duración especificada
	time.Sleep(config.Duration)
	close(done)
	wg.Wait()
	close(monitorDone)

	result.EndTime = time.Now()
	result.Operations = operations
	result.Errors = int(errors)
	result.PeakMemory = peakMemory

	// Obtener estado final
	finalMetrics, err := GetRAMMetrics()
	if err == nil {
		result.FinalMemory = finalMetrics
	}

	// Calcular métricas
	duration := result.EndTime.Sub(result.StartTime).Seconds()
	if duration > 0 {
		// Throughput: operaciones por segundo convertidas a MB/s aproximado
		bytesProcessed := float64(operations) * float64(config.BlockSize)
		result.ThroughputMBs = bytesProcessed / (1024 * 1024) / duration
	}

	// Verificar uso de swap
	result.SwapUsed = result.FinalMemory.SwapUsed > result.InitialMemory.SwapUsed

	// Calcular degradación (comparar rendimiento inicial vs final)
	if result.InitialMemory.UsagePercent > 0 {
		initialThroughput := 100.0 // Baseline teórico
		if result.ThroughputMBs > 0 {
			result.Degradation = ((initialThroughput - result.ThroughputMBs) / initialThroughput) * 100
			if result.Degradation < 0 {
				result.Degradation = 0
			}
		}
	}

	// Calcular puntuación de estabilidad
	result.StabilityScore = calculateStabilityScore(result)

	// Recopilar reporte de aislamiento si se estaba validando
	if isolationReportChan != nil {
		select {
		case isolationReport := <-isolationReportChan:
			result.IsolationReport = isolationReport
		case <-time.After(1 * time.Second):
			// Timeout, no esperar más
		}
	}

	return result, nil
}

// calculateStabilityScore calcula una puntuación de estabilidad (0-100)
func calculateStabilityScore(result *RAMStressResult) float64 {
	score := 100.0

	// Penalizar por uso de swap
	if result.SwapUsed {
		score -= 30.0
	}

	// Penalizar por degradación alta
	if result.Degradation > 50 {
		score -= 30.0
	} else if result.Degradation > 25 {
		score -= 15.0
	} else if result.Degradation > 10 {
		score -= 5.0
	}

	// Penalizar por errores
	if result.Errors > 0 {
		score -= float64(result.Errors) * 2.0
	}

	// Penalizar si el uso de memoria final es muy alto
	if result.FinalMemory.UsagePercent > 95 {
		score -= 20.0
	} else if result.FinalMemory.UsagePercent > 85 {
		score -= 10.0
	}

	// Asegurar que el score esté en el rango 0-100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}
