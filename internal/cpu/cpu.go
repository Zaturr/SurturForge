package cpu

import (
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
)

func GetCPUInfo() (string, error) {
	var info strings.Builder

	// Obtener información de CPU
	cpuInfos, err := cpu.Info()
	if err != nil {
		return "", fmt.Errorf("error al obtener información de CPU: %w", err)
	}

	if len(cpuInfos) == 0 {
		return "", fmt.Errorf("no se encontró información de CPU")
	}

	// Usar la primera CPU (generalmente todas tienen la misma información básica)
	infoObj := cpuInfos[0]

	// Construir la información formateada
	if infoObj.ModelName != "" {
		info.WriteString(fmt.Sprintf("Modelo: %s\n", infoObj.ModelName))
	}
	if infoObj.VendorID != "" {
		info.WriteString(fmt.Sprintf("Fabricante: %s\n", infoObj.VendorID))
	}
	if infoObj.Family != "" {
		info.WriteString(fmt.Sprintf("Familia: %s\n", infoObj.Family))
	}
	if infoObj.Model != "" {
		info.WriteString(fmt.Sprintf("Modelo ID: %s\n", infoObj.Model))
	}
	if infoObj.Stepping != 0 {
		info.WriteString(fmt.Sprintf("Stepping: %d\n", infoObj.Stepping))
	}

	// Obtener número de cores físicos y lógicos
	physicalCores, err := cpu.Counts(false)
	if err == nil {
		info.WriteString(fmt.Sprintf("Cores Físicos: %d\n", physicalCores))
	}

	logicalCores, err := cpu.Counts(true)
	if err == nil {
		info.WriteString(fmt.Sprintf("Cores Lógicos: %d\n", logicalCores))
	}

	// Obtener frecuencia de CPU si está disponible
	if infoObj.Mhz > 0 {
		info.WriteString(fmt.Sprintf("Frecuencia: %.2f MHz\n", infoObj.Mhz))
	}

	// Información adicional de CPU
	if infoObj.CacheSize > 0 {
		info.WriteString(fmt.Sprintf("Tamaño de Cache: %d KB\n", infoObj.CacheSize))
	}
	if infoObj.PhysicalID != "" {
		info.WriteString(fmt.Sprintf("ID Físico: %s\n", infoObj.PhysicalID))
	}
	if infoObj.CoreID != "" {
		info.WriteString(fmt.Sprintf("ID de Core: %s\n", infoObj.CoreID))
	}
	if infoObj.Microcode != "" {
		info.WriteString(fmt.Sprintf("Microcódigo: %s\n", infoObj.Microcode))
	}
	if len(infoObj.Flags) > 0 {
		maxFlags := 10
		if len(infoObj.Flags) < maxFlags {
			maxFlags = len(infoObj.Flags)
		}
		info.WriteString(fmt.Sprintf("Flags de CPU: %s\n", strings.Join(infoObj.Flags[:maxFlags], ", ")))
		if len(infoObj.Flags) > 10 {
			info.WriteString(fmt.Sprintf("... y %d más\n", len(infoObj.Flags)-10))
		}
	}

	// Si hay múltiples CPUs, mostrar información adicional
	if len(cpuInfos) > 1 {
		info.WriteString(fmt.Sprintf("\nTotal de CPUs detectadas: %d\n", len(cpuInfos)))
	}

	return info.String(), nil
}
