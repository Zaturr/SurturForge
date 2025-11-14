package main

import (
	"fmt"
	"v2/internal/cpu"
	"v2/internal/disk"
)

func main() {
	fmt.Println("===Preparacion===")
	fmt.Println()

	fmt.Println("===CPU===")
	cpuInfo, err := cpu.GetCPUInfo()
	if err != nil {
		fmt.Printf("Error al obtener la informacion del CPU: %v\n", err)
		return
	}
	fmt.Println(cpuInfo)

	fmt.Println("===Disco===")
	diskInfo, err := disk.GetDiskInfo()
	if err != nil {
		fmt.Printf("Error al obtener la informacion del disco: %v\n", err)
		return
	}
	fmt.Println(diskInfo)
	diskMetric, err := disk.GetDiskMetrics()
	if err != nil {
		fmt.Printf("Error al obtener las metricas del disco: %v\n", err)
		return
	}
	fmt.Printf("Bytes Leídos: %d\n", diskMetric.BytesRead)
	fmt.Printf("Bytes Escritos: %d\n", diskMetric.BytesWritten)
	fmt.Printf("Operaciones: %d\n", diskMetric.Operations)
	fmt.Printf("Throughput: %.2f MB/s\n", diskMetric.ThroughputMBs)
	fmt.Printf("IOPS: %.2f\n", diskMetric.IOPS)
	fmt.Printf("Latencia: %v\n", diskMetric.Latency)
	fmt.Printf("Uso del Disco: %.2f%%\n", diskMetric.DiskUsage)

}
