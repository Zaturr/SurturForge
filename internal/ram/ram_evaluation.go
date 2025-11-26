package ram

import (
	"fmt"
	"time"

	"v2/internal/baseline"
)

type RAMEvaluationStrategy struct {
	LightTest   RAMStressConfig
	MediumTest  RAMStressConfig
	HeavyTest   RAMStressConfig
	ExtremeTest RAMStressConfig
}

func GetRecommendedEvaluationStrategy() RAMEvaluationStrategy {
	return RAMEvaluationStrategy{
		LightTest:   GetIntensityConfig("light"),
		MediumTest:  GetIntensityConfig("medium"),
		HeavyTest:   GetIntensityConfig("heavy"),
		ExtremeTest: GetIntensityConfig("extreme"),
	}
}

func RunFullRAMEvaluation(strategy RAMEvaluationStrategy, baselineResult *baseline.BaselineResult) (*FullRAMEvaluationResult, error) {
	result := &FullRAMEvaluationResult{
		StartTime: time.Now(),
		Strategy:  strategy,
	}

	initialMetrics, err := GetRAMMetrics()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo métricas iniciales: %w", err)
	}
	result.InitialState = initialMetrics

	if initialMetrics.AvailableRAM < 100*1024*1024 { // Menos de 100MB
		return nil, fmt.Errorf("memoria disponible insuficiente para realizar tests (disponible: %s)",
			formatBytes(initialMetrics.AvailableRAM))
	}

	fmt.Println("Fase 1: Ejecutando test ligero de RAM...")
	lightResult, err := RunRAMStress(strategy.LightTest, baselineResult)
	if err != nil {
		return nil, fmt.Errorf("error en test ligero: %w", err)
	}
	result.LightTestResult = lightResult

	if lightResult.SwapUsed || lightResult.Degradation > 50 {
		result.Warnings = append(result.Warnings,
			"El test ligero detectó problemas. Se recomienda no ejecutar tests más intensos hasta resolverlos.")
		result.EndTime = time.Now()
		return result, nil
	}

	time.Sleep(2 * time.Second)

	fmt.Println("Fase 2: Ejecutando test medio de RAM...")
	mediumResult, err := RunRAMStress(strategy.MediumTest, baselineResult)
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Error en test medio: %v", err))
	} else {
		result.MediumTestResult = mediumResult
	}

	if mediumResult != nil && (mediumResult.SwapUsed || mediumResult.Degradation > 40) {
		result.Warnings = append(result.Warnings,
			"El test medio detectó problemas significativos. Se recomienda no ejecutar el test pesado.")
		result.EndTime = time.Now()
		return result, nil
	}

	time.Sleep(2 * time.Second)

	currentMetrics, _ := GetRAMMetrics()
	availablePercent := float64(currentMetrics.AvailableRAM) / float64(currentMetrics.TotalRAM) * 100

	if availablePercent > 20 {
		fmt.Println("Fase 3: Ejecutando test pesado de RAM...")
		heavyResult, err := RunRAMStress(strategy.HeavyTest, baselineResult)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Error en test pesado: %v", err))
		} else {
			result.HeavyTestResult = heavyResult
		}
	} else {
		result.Warnings = append(result.Warnings,
			"No se ejecutó el test pesado debido a memoria disponible insuficiente.")
	}

	finalMetrics, _ := GetRAMMetrics()
	result.FinalState = finalMetrics
	result.EndTime = time.Now()

	return result, nil
}

type FullRAMEvaluationResult struct {
	StartTime         time.Time
	EndTime           time.Time
	Strategy          RAMEvaluationStrategy
	InitialState      RAMMetrics
	FinalState        RAMMetrics
	LightTestResult   *RAMStressResult
	MediumTestResult  *RAMStressResult
	HeavyTestResult   *RAMStressResult
	ExtremeTestResult *RAMStressResult
	Warnings          []string
	Recommendations   []string
}

func (r *FullRAMEvaluationResult) GetBestResult() *RAMStressResult {
	if r.HeavyTestResult != nil {
		return r.HeavyTestResult
	}
	if r.MediumTestResult != nil {
		return r.MediumTestResult
	}
	return r.LightTestResult
}

func GetRecommendedRAMThresholds() RAMThresholds {
	return RAMThresholds{
		ExcellentUsagePercent: 60.0,
		GoodUsagePercent:      75.0,
		FairUsagePercent:      85.0,
		PoorUsagePercent:      95.0,

		ExcellentStabilityScore: 90.0,
		GoodStabilityScore:      75.0,
		FairStabilityScore:      60.0,
		PoorStabilityScore:      40.0,

		ExcellentDegradation: 5.0,
		GoodDegradation:      15.0,
		FairDegradation:      30.0,
		PoorDegradation:      50.0,
	}
}

type RAMThresholds struct {
	ExcellentUsagePercent float64
	GoodUsagePercent      float64
	FairUsagePercent      float64
	PoorUsagePercent      float64

	ExcellentStabilityScore float64
	GoodStabilityScore      float64
	FairStabilityScore      float64
	PoorStabilityScore      float64

	ExcellentDegradation float64
	GoodDegradation      float64
	FairDegradation      float64
	PoorDegradation      float64
}

func EvaluateRAMAgainstThresholds(result *RAMStressResult) *RAMThresholdEvaluation {
	evaluation := &RAMThresholdEvaluation{
		Thresholds: GetRecommendedRAMThresholds(),
	}

	if result == nil {
		return evaluation
	}

	usagePercent := result.FinalMemory.UsagePercent
	switch {
	case usagePercent < evaluation.Thresholds.ExcellentUsagePercent:
		evaluation.UsageLevel = "excellent"
	case usagePercent < evaluation.Thresholds.GoodUsagePercent:
		evaluation.UsageLevel = "good"
	case usagePercent < evaluation.Thresholds.FairUsagePercent:
		evaluation.UsageLevel = "fair"
	case usagePercent < evaluation.Thresholds.PoorUsagePercent:
		evaluation.UsageLevel = "poor"
	default:
		evaluation.UsageLevel = "critical"
	}

	switch {
	case result.StabilityScore >= evaluation.Thresholds.ExcellentStabilityScore:
		evaluation.StabilityLevel = "excellent"
	case result.StabilityScore >= evaluation.Thresholds.GoodStabilityScore:
		evaluation.StabilityLevel = "good"
	case result.StabilityScore >= evaluation.Thresholds.FairStabilityScore:
		evaluation.StabilityLevel = "fair"
	case result.StabilityScore >= evaluation.Thresholds.PoorStabilityScore:
		evaluation.StabilityLevel = "poor"
	default:
		evaluation.StabilityLevel = "critical"
	}

	switch {
	case result.Degradation < evaluation.Thresholds.ExcellentDegradation:
		evaluation.DegradationLevel = "excellent"
	case result.Degradation < evaluation.Thresholds.GoodDegradation:
		evaluation.DegradationLevel = "good"
	case result.Degradation < evaluation.Thresholds.FairDegradation:
		evaluation.DegradationLevel = "fair"
	case result.Degradation < evaluation.Thresholds.PoorDegradation:
		evaluation.DegradationLevel = "poor"
	default:
		evaluation.DegradationLevel = "critical"
	}

	return evaluation
}

type RAMThresholdEvaluation struct {
	Thresholds       RAMThresholds
	UsageLevel       string
	StabilityLevel   string
	DegradationLevel string
}
