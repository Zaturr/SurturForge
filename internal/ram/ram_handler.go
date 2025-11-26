package ram

import (
	"fmt"

	"v2/internal/baseline"
)

func DisplayRAMStressResult(result *RAMStressResult, colorReset, colorRed string, printHeader func(string), printError func(string)) {
	if result == nil {
		return
	}

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

	if result.Optimizations != nil {
		fmt.Println("\nOptimizaciones de Aislamiento:")
		if result.Optimizations.MemoryLocked {
			fmt.Println("  ✓ Memoria bloqueada en RAM (mlock)")
		} else if result.Config.EnableMemoryLock {
			fmt.Printf("  ✗ Memoria no bloqueada: %s\n", result.Optimizations.MemoryLockError)
		}
		if result.Optimizations.HighPriority {
			fmt.Println("  ✓ Prioridad alta aplicada")
		} else if result.Config.EnableHighPriority {
			fmt.Printf("  ✗ Prioridad alta no aplicada: %s\n", result.Optimizations.PriorityError)
		}
	}
}

func DisplayFullRAMEvaluation(evaluation *FullRAMEvaluationResult, printHeader func(string)) {
	if evaluation == nil {
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
	if evaluation.ExtremeTestResult != nil {
		fmt.Printf("Extremo: %.2f MB/s | Deg: %.1f%%\n",
			evaluation.ExtremeTestResult.ThroughputMBs,
			evaluation.ExtremeTestResult.Degradation)
	}
}

func RunFullRAMEvaluationWithDisplay(baselineResult *baseline.BaselineResult, printHeader, printInfo, printError func(string)) error {
	printHeader("EVALUACIÓN RAM")
	printInfo("Ejecutando...")

	strategy := GetRecommendedEvaluationStrategy()
	evaluation, err := RunFullRAMEvaluation(strategy, baselineResult)
	if err != nil {
		printError(fmt.Sprintf("Error: %v", err))
		return err
	}

	DisplayFullRAMEvaluation(evaluation, printHeader)
	return nil
}
