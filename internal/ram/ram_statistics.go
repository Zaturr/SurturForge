package ram

import (
	"math"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type ExtendedStats struct {
	Median    float64
	P95       float64
	P99       float64
	IQR       float64
	CI95Lower float64
	CI95Upper float64
	Outliers  []int
}

type EnvironmentInfo struct {
	GoVersion      string
	OS             string
	Arch           string
	CPUCores       int
	SystemLoadAvg  float64
	MemoryLoad     float64
	CPUUsageBefore float64
}

type TemporalStability struct {
	TrendSlope     float64
	TrendStrength  float64
	FirstHalfAvg   float64
	SecondHalfAvg  float64
	StabilityScore float64
}

func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}

	index := percentile * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sortedValues[lower]
	}

	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

func calculateExtendedStats(values []float64) ExtendedStats {
	stats := ExtendedStats{}

	if len(values) == 0 {
		return stats
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	stats.Median = calculatePercentile(sorted, 0.50)

	stats.P95 = calculatePercentile(sorted, 0.95)
	stats.P99 = calculatePercentile(sorted, 0.99)

	q1 := calculatePercentile(sorted, 0.25)
	q3 := calculatePercentile(sorted, 0.75)
	stats.IQR = q3 - q1

	if len(values) > 1 {
		mean := calculateAverage(values)
		stdDev := calculateStdDev(values)
		n := float64(len(values))

		tValue := 1.96
		if n < 30 {
			tValue = 2.0 + (30.0-n)/30.0*0.04
		}

		margin := tValue * stdDev / math.Sqrt(n)
		stats.CI95Lower = mean - margin
		stats.CI95Upper = mean + margin
	}

	if stats.IQR > 0 {
		lowerBound := q1 - 1.5*stats.IQR
		upperBound := q3 + 1.5*stats.IQR

		for i, v := range values {
			if v < lowerBound || v > upperBound {
				stats.Outliers = append(stats.Outliers, i+1)
			}
		}
	}

	return stats
}

func calculateTemporalStability(values []float64) TemporalStability {
	stability := TemporalStability{}

	if len(values) < 2 {
		return stability
	}

	// Dividir en dos mitades
	mid := len(values) / 2
	firstHalf := values[:mid]
	secondHalf := values[mid:]

	stability.FirstHalfAvg = calculateAverage(firstHalf)
	stability.SecondHalfAvg = calculateAverage(secondHalf)

	// Calcular tendencia usando regresión lineal simple
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(values))

	for i, v := range values {
		x := float64(i + 1)
		sumX += x
		sumY += v
		sumXY += x * v
		sumX2 += x * x
	}

	// Pendiente de la línea de tendencia
	denominator := n*sumX2 - sumX*sumX
	if denominator != 0 {
		stability.TrendSlope = (n*sumXY - sumX*sumY) / denominator
	}

	// Calcular fuerza de la tendencia (coeficiente de correlación)
	meanX := sumX / n
	meanY := sumY / n
	var sumSqX, sumSqY, sumSqXY float64

	for i, v := range values {
		x := float64(i + 1)
		dx := x - meanX
		dy := v - meanY
		sumSqX += dx * dx
		sumSqY += dy * dy
		sumSqXY += dx * dy
	}

	if sumSqX > 0 && sumSqY > 0 {
		correlation := sumSqXY / math.Sqrt(sumSqX*sumSqY)
		stability.TrendStrength = math.Abs(correlation)
	}

	if stability.FirstHalfAvg > 0 {
		variation := math.Abs(stability.SecondHalfAvg-stability.FirstHalfAvg) / stability.FirstHalfAvg
		stability.StabilityScore = math.Max(0, 100.0*(1.0-variation*2.0))
	} else {
		stability.StabilityScore = 100.0
	}

	return stability
}

// getEnvironmentInfo obtiene información del entorno del sistema
func getEnvironmentInfo() EnvironmentInfo {
	info := EnvironmentInfo{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		CPUCores:  runtime.NumCPU(),
	}

	if runtime.GOOS != "windows" {
	}

	// Carga de memoria
	if vmem, err := mem.VirtualMemory(); err == nil {
		info.MemoryLoad = vmem.UsedPercent
	}

	// Uso de CPU antes del benchmark
	if percentages, err := cpu.Percent(500*time.Millisecond, false); err == nil && len(percentages) > 0 {
		info.CPUUsageBefore = percentages[0]
	}

	return info
}
