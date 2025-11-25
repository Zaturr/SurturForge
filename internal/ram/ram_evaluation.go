package ram

import (
	"fmt"
	"time"

	"v2/internal/baseline"
)

// RAMEvaluationStrategy define la estrategia recomendada para evaluar RAM
type RAMEvaluationStrategy struct {
	// Fase 1: Test ligero para verificar estado básico
	LightTest RAMStressConfig
	// Fase 2: Test medio para evaluar rendimiento normal
	MediumTest RAMStressConfig
	// Fase 3: Test pesado para encontrar límites
	HeavyTest RAMStressConfig
	// Fase 4: Test extremo (opcional, solo si el sistema lo permite)
	ExtremeTest RAMStressConfig
}

// GetRecommendedEvaluationStrategy retorna una estrategia recomendada para evaluar RAM
func GetRecommendedEvaluationStrategy() RAMEvaluationStrategy {
	return RAMEvaluationStrategy{
		LightTest:   GetIntensityConfig("light"),
		MediumTest:  GetIntensityConfig("medium"),
		HeavyTest:   GetIntensityConfig("heavy"),
		ExtremeTest: GetIntensityConfig("extreme"),
	}
}

// RunFullRAMEvaluation ejecuta una evaluación completa de RAM siguiendo la estrategia recomendada
func RunFullRAMEvaluation(strategy RAMEvaluationStrategy, baselineResult *baseline.BaselineResult) (*FullRAMEvaluationResult, error) {
	result := &FullRAMEvaluationResult{
		StartTime: time.Now(),
		Strategy:  strategy,
	}

	// Verificar estado inicial
	initialMetrics, err := GetRAMMetrics()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo métricas iniciales: %w", err)
	}
	result.InitialState = initialMetrics

	// Verificar que hay suficiente memoria disponible
	if initialMetrics.AvailableRAM < 100*1024*1024 { // Menos de 100MB
		return nil, fmt.Errorf("memoria disponible insuficiente para realizar tests (disponible: %s)",
			formatBytes(initialMetrics.AvailableRAM))
	}

	// Fase 1: Test Ligero (siempre ejecutar)
	fmt.Println("Fase 1: Ejecutando test ligero de RAM...")
	lightResult, err := RunRAMStress(strategy.LightTest, baselineResult)
	if err != nil {
		return nil, fmt.Errorf("error en test ligero: %w", err)
	}
	result.LightTestResult = lightResult

	// Si el test ligero muestra problemas, no continuar con tests más intensos
	if lightResult.SwapUsed || lightResult.Degradation > 50 {
		result.Warnings = append(result.Warnings,
			"El test ligero detectó problemas. Se recomienda no ejecutar tests más intensos hasta resolverlos.")
		result.EndTime = time.Now()
		return result, nil
	}

	// Esperar un momento para que el sistema se recupere
	time.Sleep(2 * time.Second)

	// Fase 2: Test Medio
	fmt.Println("Fase 2: Ejecutando test medio de RAM...")
	mediumResult, err := RunRAMStress(strategy.MediumTest, baselineResult)
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Error en test medio: %v", err))
	} else {
		result.MediumTestResult = mediumResult
	}

	// Si el test medio muestra problemas significativos, considerar no hacer el test pesado
	if mediumResult != nil && (mediumResult.SwapUsed || mediumResult.Degradation > 40) {
		result.Warnings = append(result.Warnings,
			"El test medio detectó problemas significativos. Se recomienda no ejecutar el test pesado.")
		result.EndTime = time.Now()
		return result, nil
	}

	// Esperar un momento para que el sistema se recupere
	time.Sleep(2 * time.Second)

	// Fase 3: Test Pesado (solo si hay suficiente memoria disponible)
	currentMetrics, _ := GetRAMMetrics()
	availablePercent := float64(currentMetrics.AvailableRAM) / float64(currentMetrics.TotalRAM) * 100

	if availablePercent > 20 { // Al menos 20% disponible
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

	// Obtener estado final
	finalMetrics, _ := GetRAMMetrics()
	result.FinalState = finalMetrics
	result.EndTime = time.Now()

	return result, nil
}

// FullRAMEvaluationResult contiene los resultados de una evaluación completa
type FullRAMEvaluationResult struct {
	StartTime        time.Time
	EndTime          time.Time
	Strategy         RAMEvaluationStrategy
	InitialState     RAMMetrics
	FinalState       RAMMetrics
	LightTestResult  *RAMStressResult
	MediumTestResult *RAMStressResult
	HeavyTestResult  *RAMStressResult
	ExtremeTestResult *RAMStressResult
	Warnings         []string
	Recommendations  []string
}

// GetBestResult retorna el resultado del test más intenso completado exitosamente
func (r *FullRAMEvaluationResult) GetBestResult() *RAMStressResult {
	if r.HeavyTestResult != nil {
		return r.HeavyTestResult
	}
	if r.MediumTestResult != nil {
		return r.MediumTestResult
	}
	return r.LightTestResult
}

// GetRecommendedRAMThresholds retorna los umbrales recomendados para evaluar el estado de RAM
func GetRecommendedRAMThresholds() RAMThresholds {
	return RAMThresholds{
		ExcellentUsagePercent: 60.0,  // < 60% uso = excelente
		GoodUsagePercent:      75.0,  // < 75% uso = bueno
		FairUsagePercent:      85.0,  // < 85% uso = aceptable
		PoorUsagePercent:      95.0,  // < 95% uso = pobre
		// > 95% = crítico

		ExcellentStabilityScore: 90.0,
		GoodStabilityScore:      75.0,
		FairStabilityScore:      60.0,
		PoorStabilityScore:      40.0,

		ExcellentDegradation: 5.0,   // < 5% degradación = excelente
		GoodDegradation:      15.0,  // < 15% degradación = bueno
		FairDegradation:      30.0,  // < 30% degradación = aceptable
		PoorDegradation:      50.0,  // < 50% degradación = pobre
	}
}

// RAMThresholds umbrales específicos para RAM
type RAMThresholds struct {
	ExcellentUsagePercent  float64
	GoodUsagePercent      float64
	FairUsagePercent       float64
	PoorUsagePercent       float64

	ExcellentStabilityScore float64
	GoodStabilityScore      float64
	FairStabilityScore      float64
	PoorStabilityScore      float64

	ExcellentDegradation float64
	GoodDegradation      float64
	FairDegradation      float64
	PoorDegradation      float64
}

// EvaluateRAMAgainstThresholds evalúa RAM contra los umbrales recomendados
func EvaluateRAMAgainstThresholds(result *RAMStressResult) *RAMThresholdEvaluation {
	evaluation := &RAMThresholdEvaluation{
		Thresholds: GetRecommendedRAMThresholds(),
	}

	if result == nil {
		return evaluation
	}

	// Evaluar uso de RAM
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

	// Evaluar estabilidad
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

	// Evaluar degradación
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

// RAMThresholdEvaluation contiene la evaluación de RAM contra los umbrales
type RAMThresholdEvaluation struct {
	Thresholds       RAMThresholds
	UsageLevel       string
	StabilityLevel   string
	DegradationLevel string
}

