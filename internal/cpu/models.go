package cpu

import (
	"time"

	"v2/internal/baseline"
)

type CPUMetrics struct {
	UsagePercent float64
	ClockSpeed   float64
	Temperature  float64
	Cores        int
	Threads      int
	Duration     time.Time
}

type CPUStats struct {
	Min, Max, Average float64
	Samples           int
	TemperatureMin    float64
	TemperatureMax    float64
	TemperatureAvg    float64
	ClockSpeedMin     float64
	ClockSpeedMax     float64
	ClockSpeedAvg     float64
	EnergyConsumption float64
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
}

type BenchmarkResult struct {
	Name       string
	Iterations string
	TimePerOp  string
	Allocs     string
	Bytes      string
}

type BenchmarkReport struct {
	Benchmarks      map[string]*BenchmarkResult
	SingleCoreScore float64
	MultiCoreScore  float64
	Stats           *CPUStats
	BaselineResult  *baseline.BaselineResult
	// Métricas de CPU durante el benchmark
	CPUUsageBefore  float64
	CPUUsageAfter   float64
	CPUUsageChange  float64
	// Métricas de rendimiento
	MIPS            float64 // Million Instructions Per Second
	GIPS            float64 // Giga Instructions Per Second
	FLOPS           float64 // Floating Point Operations Per Second
}

