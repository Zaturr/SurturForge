package disk

import (
	"os"
	"path/filepath"
	"time"
)

// ExampleDiskStress muestra un ejemplo de cómo usar el handler de estrés del disco
func ExampleDiskStress() {
	// Configurar el test de estrés
	config := DiskStressConfig{
		DBPath:             filepath.Join(os.TempDir(), "disk_stress_test.db"),
		WriteWorkers:       4,
		ReadWorkers:        4,
		OperationsPerCycle: 100,
		TestDuration:       30 * time.Second,
		DataSize:           1024,
	}

	// Ejecutar el test de estrés
	result, err := RunDiskStress(config)
	if err != nil {
		panic(err)
	}

	// Imprimir los resultados
	PrintDiskStressResult(result)
}
