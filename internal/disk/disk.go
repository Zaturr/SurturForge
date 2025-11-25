package disk

import (
	"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

type DiskMetrics struct {
	Duration       time.Time
	BytesRead      int64
	BytesWritten   int64
	Operations     int64
	ThroughputMBs  float64
	IOPS           float64
	Latency        time.Duration
	DiskUsage      float64
	ReadTime       uint64
	WriteTime      uint64
	IoTime         uint64
	WeightedIO     uint64
	IopsInProgress uint64
}

type ioStats struct {
	BytesRead      int64
	BytesWritten   int64
	Operations     int64
	ReadTime       uint64
	WriteTime      uint64
	IoTime         uint64
	WeightedIO     uint64
	IopsInProgress uint64
}

func GetDiskMetrics() (DiskMetrics, error) {
	metrics := DiskMetrics{
		Duration: time.Now(),
	}

	initialIO, err := getIOStats()
	if err != nil {
		return metrics, err
	}

	time.Sleep(1 * time.Second)

	finalIO, err := getIOStats()
	if err != nil {
		return metrics, err
	}

	duration := 1.0
	bytesRead := finalIO.BytesRead - initialIO.BytesRead
	bytesWritten := finalIO.BytesWritten - initialIO.BytesWritten
	operations := finalIO.Operations - initialIO.Operations

	metrics.BytesRead = bytesRead
	metrics.BytesWritten = bytesWritten
	metrics.Operations = operations
	metrics.ThroughputMBs = float64(bytesRead+bytesWritten) / (1024 * 1024) / duration
	metrics.IOPS = float64(operations) / duration
	metrics.ReadTime = finalIO.ReadTime - initialIO.ReadTime
	metrics.WriteTime = finalIO.WriteTime - initialIO.WriteTime
	metrics.IoTime = finalIO.IoTime - initialIO.IoTime
	metrics.WeightedIO = finalIO.WeightedIO - initialIO.WeightedIO
	metrics.IopsInProgress = finalIO.IopsInProgress

	if operations > 0 {
		metrics.Latency = time.Duration(float64(duration)/float64(operations)*1000) * time.Millisecond
	}

	usage, err := getDiskUsage()
	if err == nil {
		metrics.DiskUsage = usage
	}

	return metrics, nil
}

func getIOStats() (ioStats, error) {
	var stats ioStats

	counters, err := disk.IOCounters()
	if err != nil {
		return stats, err
	}

	for _, counter := range counters {
		stats.BytesRead += int64(counter.ReadBytes)
		stats.BytesWritten += int64(counter.WriteBytes)
		stats.Operations += int64(counter.ReadCount + counter.WriteCount)
		stats.ReadTime += counter.ReadTime
		stats.WriteTime += counter.WriteTime
		stats.IoTime += counter.IoTime
		stats.WeightedIO += counter.WeightedIO
		stats.IopsInProgress += counter.IopsInProgress
	}

	return stats, nil
}

func getDiskUsage() (float64, error) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return 0, err
	}

	var totalUsage float64
	var count int

	for _, partition := range partitions {
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			continue
		}
		totalUsage += usage.UsedPercent
		count++
	}

	if count == 0 {
		return 0, fmt.Errorf("no se encontraron particiones montadas")
	}

	return totalUsage / float64(count), nil
}

func GetDiskInfo() (string, error) {
	var info strings.Builder

	partitions, err := disk.Partitions(false)
	if err != nil {
		return "", fmt.Errorf("error al obtener particiones: %w", err)
	}

	if len(partitions) == 0 {
		return "", fmt.Errorf("no se encontraron particiones")
	}

	for _, partition := range partitions {
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			continue
		}

		info.WriteString(fmt.Sprintf("\nDispositivo: %s\n", partition.Device))
		info.WriteString(fmt.Sprintf("Punto de Montaje: %s\n", partition.Mountpoint))
		info.WriteString(fmt.Sprintf("Tipo de Sistema de Archivos: %s\n", partition.Fstype))

		info.WriteString(fmt.Sprintf("Tamaño Total: %s\n", formatBytes(usage.Total)))
		info.WriteString(fmt.Sprintf("Espacio Usado: %s\n", formatBytes(usage.Used)))
		info.WriteString(fmt.Sprintf("Espacio Libre: %s\n", formatBytes(usage.Free)))
		info.WriteString(fmt.Sprintf("Uso: %.2f%%\n", usage.UsedPercent))
		if usage.InodesTotal > 0 {
			info.WriteString(fmt.Sprintf("Inodos Total: %d\n", usage.InodesTotal))
			info.WriteString(fmt.Sprintf("Inodos Usados: %d\n", usage.InodesUsed))
			info.WriteString(fmt.Sprintf("Inodos Libres: %d\n", usage.InodesFree))
			info.WriteString(fmt.Sprintf("Uso de Inodos: %.2f%%\n", usage.InodesUsedPercent))
		}
		info.WriteString("\n")
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
