package cpu

import (
	"time"

	"v2/internal/baseline"
)

type CPUMetrics struct {
	UsagePercent float64 // porcentaje de uso
	ClockSpeed   float64 // MHz
	Temperature  float64 // Celsius (si está disponible)
	Cores        int     // número de cores físicos
	Threads      int     // número de threads lógicos
	Duration     time.Time
}

// CPUStats contiene estadísticas de CPU durante un benchmark
type CPUStats struct {
	Min, Max, Average float64
	Samples           int
	// Métricas adicionales
	TemperatureMin    float64
	TemperatureMax    float64
	TemperatureAvg    float64
	ClockSpeedMin     float64
	ClockSpeedMax     float64
	ClockSpeedAvg     float64
	EnergyConsumption float64 // Watts estimados
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
}

// BenchmarkResult representa el resultado de un benchmark
type BenchmarkResult struct {
	Name       string
	Iterations string
	TimePerOp  string
	Allocs     string
	Bytes      string
}

// BenchmarkReport contiene todos los datos del reporte de benchmarks
type BenchmarkReport struct {
	Benchmarks      map[string]*BenchmarkResult
	SingleCoreScore float64
	MultiCoreScore  float64
	Stats           *CPUStats
	BaselineResult  *baseline.BaselineResult
}

