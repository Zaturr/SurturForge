package baseline

import (
	"fmt"
	"os"
)

// ExampleRunBaseline muestra cómo usar el paquete baseline
func ExampleRunBaseline() {
	// Establecer variables de entorno recomendadas
	if err := SetEnvironmentVariables(); err != nil {
		fmt.Printf("Advertencia: No se pudieron establecer variables de entorno: %v\n", err)
	}

	// Verificar si se ejecuta con privilegios de administrador
	isAdmin := CheckAdminRights()
	if !isAdmin {
		fmt.Println("Advertencia: El programa no se está ejecutando con privilegios de administrador.")
		fmt.Println("Algunas mediciones pueden ser menos precisas.")
		fmt.Println()
	}

	// Ejecutar el baseline completo
	fmt.Println("Ejecutando baseline del sistema...")
	fmt.Println("Esto puede tomar unos segundos...")
	fmt.Println()

	result, err := RunBaseline()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ejecutando baseline: %v\n", err)
		return
	}

	// Imprimir los resultados
	PrintBaseline(result)

	// Ejemplo de uso individual de las funciones
	fmt.Println("\n=== USO INDIVIDUAL DE FUNCIONES ===")
	
	// Solo detección de hardware
	hardware, err := DetectHardware()
	if err == nil {
		fmt.Printf("Hardware detectado: %d cores físicos, %d cores lógicos\n",
			hardware.CPUCoresPhysical, hardware.CPUCoresLogical)
	}

	// Solo baseline del sistema
	baseline, err := EstablishBaseline()
	if err == nil {
		fmt.Printf("CPU Idle: %.2f%%, Memoria Libre: %s\n",
			baseline.CPUIdlePercent, FormatBytes(baseline.MemoryFree))
	}

	// Solo configuración del entorno
	env, err := ConfigureEnvironment()
	if err == nil {
		fmt.Printf("Sistema: %s %s, Procesos: %d\n",
			env.OS, env.Architecture, env.ProcessCount)
	}
}

