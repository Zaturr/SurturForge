package ram

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"v2/internal/baseline"

	"github.com/shirou/gopsutil/v4/mem"
)

type RAMStressConfig struct {
	MemoryPercentage   float64
	Duration           time.Duration
	Workers            int
	BlockSize          int
	OperationType      string
	Intensity          string
	EnableMemoryLock   bool
	EnableHighPriority bool
}

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
	Degradation     float64
	StabilityScore  float64
	IsolationReport *IsolationReport
	Optimizations   *OptimizationResult
}

func DefaultRAMStressConfig() RAMStressConfig {
	availablePercent := 0.1

	return RAMStressConfig{
		MemoryPercentage:   availablePercent,
		Duration:           30 * time.Second,
		Workers:            runtime.NumCPU(),
		BlockSize:          1024 * 1024,
		OperationType:      "mixed",
		Intensity:          "medium",
		EnableMemoryLock:   false,
		EnableHighPriority: false,
	}
}

func EnableIsolationOptimizations(config *RAMStressConfig) {
	config.EnableMemoryLock = true
	config.EnableHighPriority = true
}

func GetIntensityConfig(intensity string) RAMStressConfig {
	config := DefaultRAMStressConfig()
	config.Intensity = intensity

	switch intensity {
	case "light":
		config.MemoryPercentage = 0.05
		config.Duration = 15 * time.Second
		config.Workers = runtime.NumCPU() / 2
		config.BlockSize = 512 * 1024
		config.OperationType = "read"

	case "medium":
		config.MemoryPercentage = 0.15
		config.Duration = 30 * time.Second
		config.Workers = runtime.NumCPU()
		config.BlockSize = 1024 * 1024
		config.OperationType = "mixed"

	case "heavy":
		config.MemoryPercentage = 0.30
		config.Duration = 60 * time.Second
		config.Workers = runtime.NumCPU() * 2
		config.BlockSize = 2 * 1024 * 1024
		config.OperationType = "mixed"

	case "extreme":
		config.MemoryPercentage = 0.50
		config.Duration = 120 * time.Second
		config.Workers = runtime.NumCPU() * 4
		config.BlockSize = 4 * 1024 * 1024
		config.OperationType = "mixed"
	}

	return config
}

func RunRAMStress(config RAMStressConfig, baselineResult ...*baseline.BaselineResult) (*RAMStressResult, error) {
	result := &RAMStressResult{
		Config:    config,
		StartTime: time.Now(),
	}

	initialMetrics, err := GetRAMMetrics()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo métricas iniciales: %w", err)
	}
	result.InitialMemory = initialMetrics

	var baselineForIsolation *baseline.BaselineResult
	if len(baselineResult) > 0 && baselineResult[0] != nil {
		baselineForIsolation = baselineResult[0]
	}

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

	vmem, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo información de memoria: %w", err)
	}

	availableBytes := uint64(float64(vmem.Available) * config.MemoryPercentage)
	if availableBytes < uint64(config.BlockSize) {
		availableBytes = uint64(config.BlockSize) * 10
	}

	buffers := make([][]byte, config.Workers)
	for i := range buffers {
		buffers[i] = make([]byte, config.BlockSize)
	}

	optimizationResult := ApplyOptimizations(config, buffers)

	defer CleanupOptimizations(config, buffers)

	done := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var operations int64
	var errors int64

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

	operationTicker := time.NewTicker(10 * time.Millisecond)
	defer operationTicker.Stop()

	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			buffer := buffers[workerID]
			localOps := int64(0)
			pattern := byte(workerID % 256)

			for {
				select {
				case <-done:
					mu.Lock()
					operations += localOps
					mu.Unlock()
					return
				case <-operationTicker.C:
					switch config.OperationType {
					case "read":
						for j := 0; j < len(buffer); j += 1024 {
							_ = buffer[j]
						}
					case "write":
						for j := 0; j < len(buffer); j += 4 {
							buffer[j] = pattern
							if j+1 < len(buffer) {
								buffer[j+1] = pattern
							}
							if j+2 < len(buffer) {
								buffer[j+2] = pattern
							}
							if j+3 < len(buffer) {
								buffer[j+3] = pattern
							}
						}
					case "mixed":
						if localOps%2 == 0 {
							for j := 0; j < len(buffer); j += 1024 {
								_ = buffer[j]
							}
						} else {
							for j := 0; j < len(buffer); j += 4 {
								buffer[j] = pattern
								if j+1 < len(buffer) {
									buffer[j+1] = pattern
								}
								if j+2 < len(buffer) {
									buffer[j+2] = pattern
								}
								if j+3 < len(buffer) {
									buffer[j+3] = pattern
								}
							}
						}
					}
					localOps++
					runtime.Gosched()
				}
			}
		}(i)
	}

	time.Sleep(config.Duration)
	close(done)
	wg.Wait()
	close(monitorDone)

	result.EndTime = time.Now()
	result.Operations = operations
	result.Errors = int(errors)
	result.PeakMemory = peakMemory

	finalMetrics, err := GetRAMMetrics()
	if err == nil {
		result.FinalMemory = finalMetrics
	}

	duration := result.EndTime.Sub(result.StartTime).Seconds()
	if duration > 0 {
		bytesProcessed := float64(operations) * float64(config.BlockSize)
		result.ThroughputMBs = bytesProcessed / (1024 * 1024) / duration
	}

	result.SwapUsed = result.FinalMemory.SwapUsed > result.InitialMemory.SwapUsed

	if result.InitialMemory.UsagePercent > 0 {
		initialThroughput := 100.0
		if result.ThroughputMBs > 0 {
			result.Degradation = ((initialThroughput - result.ThroughputMBs) / initialThroughput) * 100
			if result.Degradation < 0 {
				result.Degradation = 0
			}
		}
	}

	result.StabilityScore = calculateStabilityScore(result)

	if isolationReportChan != nil {
		select {
		case isolationReport := <-isolationReportChan:
			result.IsolationReport = isolationReport
		case <-time.After(1 * time.Second):
		}
	}

	result.Optimizations = optimizationResult

	return result, nil
}

func calculateStabilityScore(result *RAMStressResult) float64 {
	score := 100.0

	if result.SwapUsed {
		score -= 30.0
	}

	if result.Degradation > 50 {
		score -= 30.0
	} else if result.Degradation > 25 {
		score -= 15.0
	} else if result.Degradation > 10 {
		score -= 5.0
	}

	if result.Errors > 0 {
		score -= float64(result.Errors) * 2.0
	}

	if result.FinalMemory.UsagePercent > 95 {
		score -= 20.0
	} else if result.FinalMemory.UsagePercent > 85 {
		score -= 10.0
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}
