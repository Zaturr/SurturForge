package main

import (
	"fmt"
	"v2/internal/cpu"
	"v2/internal/disk"
	"v2/internal/network"
	"v2/internal/ram"
)

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

func main() {
	fmt.Println("=== SurturForge Benchmark ===")
	fmt.Println()

	// CPU
	fmt.Println("=== CPU ===")
	cpuInfo, err := cpu.GetCPUInfo()
	if err != nil {
		fmt.Printf("Error al obtener la informacion del CPU: %v\n", err)
	} else {
		fmt.Println(cpuInfo)
	}

	cpuMetrics, err := cpu.GetCPUMetrics()
	if err != nil {
		fmt.Printf("Error al obtener las metricas del CPU: %v\n", err)
	} else {
		fmt.Println("\n--- Metricas de CPU ---")
		fmt.Printf("Uso del CPU: %.2f%%\n", cpuMetrics.UsagePercent)
		fmt.Printf("Velocidad del Reloj: %.2f MHz\n", cpuMetrics.ClockSpeed)
		fmt.Printf("Cores Fisicos: %d\n", cpuMetrics.Cores)
		fmt.Printf("Threads Logicos: %d\n", cpuMetrics.Threads)
		if cpuMetrics.Temperature > 0 {
			fmt.Printf("Temperatura: %.2f °C\n", cpuMetrics.Temperature)
		}
		fmt.Printf("Timestamp: %s\n", cpuMetrics.Duration.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	// RAM
	fmt.Println("=== RAM ===")
	ramInfo, err := ram.GetRAMInfo()
	if err != nil {
		fmt.Printf("Error al obtener la informacion de RAM: %v\n", err)
	} else {
		fmt.Println(ramInfo)
	}

	ramMetrics, err := ram.GetRAMMetrics()
	if err != nil {
		fmt.Printf("Error al obtener las metricas de RAM: %v\n", err)
	} else {
		fmt.Println("\n--- Metricas de RAM ---")
		fmt.Printf("RAM Total: %s\n", formatBytes(ramMetrics.TotalRAM))
		fmt.Printf("RAM Usada: %s\n", formatBytes(ramMetrics.UsedRAM))
		fmt.Printf("RAM Disponible: %s\n", formatBytes(ramMetrics.AvailableRAM))
		fmt.Printf("RAM Libre: %s\n", formatBytes(ramMetrics.FreeRAM))
		fmt.Printf("Uso de RAM: %.2f%%\n", ramMetrics.UsagePercent)
		if ramMetrics.Speed > 0 {
			fmt.Printf("Velocidad de RAM: %d MHz\n", ramMetrics.Speed)
		}
		// Datos adicionales
		fmt.Printf("RAM Activa: %s\n", formatBytes(ramMetrics.Active))
		fmt.Printf("RAM Inactiva: %s\n", formatBytes(ramMetrics.Inactive))
		fmt.Printf("RAM Cableada: %s\n", formatBytes(ramMetrics.Wired))
		fmt.Printf("Buffers: %s\n", formatBytes(ramMetrics.Buffers))
		fmt.Printf("Caché: %s\n", formatBytes(ramMetrics.Cached))
		fmt.Printf("Memoria Compartida: %s\n", formatBytes(ramMetrics.Shared))
		fmt.Printf("Slab: %s\n", formatBytes(ramMetrics.Slab))
		fmt.Printf("Tablas de Páginas: %s\n", formatBytes(ramMetrics.PageTables))
		// Información de Swap
		fmt.Printf("\nSwap Total: %s\n", formatBytes(ramMetrics.SwapTotal))
		fmt.Printf("Swap Usado: %s\n", formatBytes(ramMetrics.SwapUsed))
		fmt.Printf("Swap Libre: %s\n", formatBytes(ramMetrics.SwapFree))
		fmt.Printf("Uso de Swap: %.2f%%\n", ramMetrics.SwapUsedPercent)
		fmt.Printf("Timestamp: %s\n", ramMetrics.Duration.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	// Disco
	fmt.Println("=== DISCO ===")
	diskInfo, err := disk.GetDiskInfo()
	if err != nil {
		fmt.Printf("Error al obtener la informacion del disco: %v\n", err)
	} else {
		fmt.Println(diskInfo)
	}

	diskMetrics, err := disk.GetDiskMetrics()
	if err != nil {
		fmt.Printf("Error al obtener las metricas del disco: %v\n", err)
	} else {
		fmt.Println("\n--- Metricas de Disco ---")
		fmt.Printf("Bytes Leidos: %s\n", formatBytes(uint64(diskMetrics.BytesRead)))
		fmt.Printf("Bytes Escritos: %s\n", formatBytes(uint64(diskMetrics.BytesWritten)))
		fmt.Printf("Operaciones: %d\n", diskMetrics.Operations)
		fmt.Printf("Velocidad de Lectura/Escritura: %.2f MB/s\n", diskMetrics.ThroughputMBs)
		fmt.Printf("IOPS: %.2f\n", diskMetrics.IOPS)
		fmt.Printf("Latencia: %v\n", diskMetrics.Latency)
		fmt.Printf("Uso del Disco: %.2f%%\n", diskMetrics.DiskUsage)
		fmt.Printf("Tiempo de Lectura: %d ms\n", diskMetrics.ReadTime)
		fmt.Printf("Tiempo de Escritura: %d ms\n", diskMetrics.WriteTime)
		fmt.Printf("Tiempo Total de I/O: %d ms\n", diskMetrics.IoTime)
		fmt.Printf("I/O Ponderado: %d\n", diskMetrics.WeightedIO)
		fmt.Printf("IOPS en Progreso: %d\n", diskMetrics.IopsInProgress)
		fmt.Printf("Timestamp: %s\n", diskMetrics.Duration.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	// Red
	fmt.Println("=== RED ===")
	networkInfo, err := network.GetNetworkInfo()
	if err != nil {
		fmt.Printf("Error al obtener la informacion de red: %v\n", err)
	} else {
		fmt.Println(networkInfo)
	}

	networkMetrics, err := network.GetNetworkMetrics()
	if err != nil {
		fmt.Printf("Error al obtener las metricas de red: %v\n", err)
	} else {
		fmt.Println("\n--- Metricas de Red ---")
		fmt.Printf("Bytes Enviados: %s\n", formatBytes(networkMetrics.BytesSent))
		fmt.Printf("Bytes Recibidos: %s\n", formatBytes(networkMetrics.BytesReceived))
		fmt.Printf("Paquetes Enviados: %d\n", networkMetrics.PacketsSent)
		fmt.Printf("Paquetes Recibidos: %d\n", networkMetrics.PacketsReceived)
		fmt.Printf("Velocidad de Subida: %.2f MB/s\n", networkMetrics.UploadSpeed)
		fmt.Printf("Velocidad de Bajada: %.2f MB/s\n", networkMetrics.DownloadSpeed)
		fmt.Printf("Latencia: %v\n", networkMetrics.Latency)
		// Datos adicionales
		fmt.Printf("Errores de Recepción: %d\n", networkMetrics.Errin)
		fmt.Printf("Errores de Envío: %d\n", networkMetrics.Errout)
		fmt.Printf("Paquetes Descartados (Recepción): %d\n", networkMetrics.Dropin)
		fmt.Printf("Paquetes Descartados (Envío): %d\n", networkMetrics.Dropout)
		fmt.Printf("Errores FIFO (Recepción): %d\n", networkMetrics.Fifoin)
		fmt.Printf("Errores FIFO (Envío): %d\n", networkMetrics.Fifoout)
		fmt.Printf("Timestamp: %s\n", networkMetrics.Duration.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()

	fmt.Println("=== Benchmark Completado ===")
}
