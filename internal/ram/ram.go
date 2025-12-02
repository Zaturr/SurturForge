package ram

import (
	"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

type RAMMetrics struct {
	TotalRAM        uint64
	AvailableRAM    uint64
	UsedRAM         uint64
	UsagePercent    float64
	FreeRAM         uint64
	Speed           uint64
	Duration        time.Time
	Active          uint64
	Inactive        uint64
	Wired           uint64
	Buffers         uint64
	Cached          uint64
	Shared          uint64
	Slab            uint64
	PageTables      uint64
	SwapTotal       uint64
	SwapUsed        uint64
	SwapFree        uint64
	SwapUsedPercent float64
}

func GetRAMInfo() (string, error) {
	var info strings.Builder

	vmem, err := mem.VirtualMemory()
	if err != nil {
		return "", fmt.Errorf("error al obtener información de memoria virtual: %w", err)
	}

	info.WriteString("=== Información de Memoria Virtual ===\n")
	info.WriteString(fmt.Sprintf("RAM Total: %s\n", formatBytes(vmem.Total)))
	info.WriteString(fmt.Sprintf("RAM Disponible: %s\n", formatBytes(vmem.Available)))
	info.WriteString(fmt.Sprintf("RAM Usada: %s\n", formatBytes(vmem.Used)))
	info.WriteString(fmt.Sprintf("RAM Libre: %s\n", formatBytes(vmem.Free)))
	info.WriteString(fmt.Sprintf("Uso de RAM: %.2f%%\n", vmem.UsedPercent))

	if vmem.Active > 0 {
		info.WriteString(fmt.Sprintf("RAM Activa: %s\n", formatBytes(vmem.Active)))
	}
	if vmem.Inactive > 0 {
		info.WriteString(fmt.Sprintf("RAM Inactiva: %s\n", formatBytes(vmem.Inactive)))
	}
	if vmem.Wired > 0 {
		info.WriteString(fmt.Sprintf("RAM Cableada: %s\n", formatBytes(vmem.Wired)))
	}
	if vmem.Buffers > 0 {
		info.WriteString(fmt.Sprintf("Buffers: %s\n", formatBytes(vmem.Buffers)))
	}
	if vmem.Cached > 0 {
		info.WriteString(fmt.Sprintf("Caché: %s\n", formatBytes(vmem.Cached)))
	}
	if vmem.Shared > 0 {
		info.WriteString(fmt.Sprintf("Memoria Compartida: %s\n", formatBytes(vmem.Shared)))
	}
	if vmem.Slab > 0 {
		info.WriteString(fmt.Sprintf("Slab: %s\n", formatBytes(vmem.Slab)))
	}
	if vmem.PageTables > 0 {
		info.WriteString(fmt.Sprintf("Tablas de Páginas: %s\n", formatBytes(vmem.PageTables)))
	}

	swap, err := mem.SwapMemory()
	if err == nil {
		info.WriteString("\n=== Información de Swap ===\n")
		info.WriteString(fmt.Sprintf("Swap Total: %s\n", formatBytes(swap.Total)))
		info.WriteString(fmt.Sprintf("Swap Usado: %s\n", formatBytes(swap.Used)))
		info.WriteString(fmt.Sprintf("Swap Libre: %s\n", formatBytes(swap.Free)))
		info.WriteString(fmt.Sprintf("Uso de Swap: %.2f%%\n", swap.UsedPercent))
		if swap.Sin > 0 {
			info.WriteString(fmt.Sprintf("Swap In: %d páginas\n", swap.Sin))
		}
		if swap.Sout > 0 {
			info.WriteString(fmt.Sprintf("Swap Out: %d páginas\n", swap.Sout))
		}
	}

	return info.String(), nil
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func GetRAMMetrics() (RAMMetrics, error) {
	metrics := RAMMetrics{
		Duration: time.Now(),
	}

	vmem, err := mem.VirtualMemory()
	if err != nil {
		return metrics, fmt.Errorf("error al obtener métricas de memoria virtual: %w", err)
	}

	metrics.TotalRAM = vmem.Total
	metrics.AvailableRAM = vmem.Available
	metrics.UsedRAM = vmem.Used
	metrics.FreeRAM = vmem.Free
	metrics.UsagePercent = vmem.UsedPercent

	metrics.Active = vmem.Active
	metrics.Inactive = vmem.Inactive
	metrics.Wired = vmem.Wired
	metrics.Buffers = vmem.Buffers
	metrics.Cached = vmem.Cached
	metrics.Shared = vmem.Shared
	metrics.Slab = vmem.Slab
	metrics.PageTables = vmem.PageTables

	swap, err := mem.SwapMemory()
	if err == nil {
		metrics.SwapTotal = swap.Total
		metrics.SwapUsed = swap.Used
		metrics.SwapFree = swap.Free
		metrics.SwapUsedPercent = swap.UsedPercent
	}

	return metrics, nil
}
