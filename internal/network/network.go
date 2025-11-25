package network

import (
	"fmt"
	"net"
	"strings"
	"time"

	gopsutilnet "github.com/shirou/gopsutil/v4/net"
)

type NetworkMetrics struct {
	BytesSent       uint64
	BytesReceived   uint64
	PacketsSent     uint64
	PacketsReceived uint64
	UploadSpeed     float64
	DownloadSpeed   float64
	Latency         time.Duration
	Duration        time.Time

	Errin   uint64
	Errout  uint64
	Dropin  uint64
	Dropout uint64
	Fifoin  uint64
	Fifoout uint64
}

type networkStats struct {
	BytesSent       uint64
	BytesReceived   uint64
	PacketsSent     uint64
	PacketsReceived uint64
	Errin           uint64
	Errout          uint64
	Dropin          uint64
	Dropout         uint64
	Fifoin          uint64
	Fifoout         uint64
}

func GetNetworkInfo() (string, error) {
	var info strings.Builder

	interfaces, err := gopsutilnet.Interfaces()
	if err != nil {
		return "", fmt.Errorf("error al obtener interfaces de red: %w", err)
	}

	if len(interfaces) == 0 {
		return "", fmt.Errorf("no se encontraron interfaces de red")
	}

	info.WriteString("=== Información de Interfaces de Red ===\n")
	for _, iface := range interfaces {
		info.WriteString(fmt.Sprintf("\nNombre: %s\n", iface.Name))
		info.WriteString(fmt.Sprintf("Índice: %d\n", iface.Index))
		info.WriteString(fmt.Sprintf("MTU: %d\n", iface.MTU))
		if iface.HardwareAddr != "" {
			info.WriteString(fmt.Sprintf("Dirección MAC: %s\n", iface.HardwareAddr))
		}
		if len(iface.Flags) > 0 {
			info.WriteString(fmt.Sprintf("Flags: %s\n", strings.Join(iface.Flags, ", ")))
		}
		if len(iface.Addrs) > 0 {
			info.WriteString("Direcciones IP:\n")
			for _, addr := range iface.Addrs {
				info.WriteString(fmt.Sprintf("  - %s\n", addr.Addr))
			}
		}
	}

	return info.String(), nil
}

func GetNetworkMetrics() (NetworkMetrics, error) {
	metrics := NetworkMetrics{
		Duration: time.Now(),
	}

	initialStats, err := getNetworkStats()
	if err != nil {
		return metrics, err
	}

	time.Sleep(1 * time.Second)

	finalStats, err := getNetworkStats()
	if err != nil {
		return metrics, err
	}

	duration := 1.0
	bytesSent := finalStats.BytesSent - initialStats.BytesSent
	bytesReceived := finalStats.BytesReceived - initialStats.BytesReceived
	packetsSent := finalStats.PacketsSent - initialStats.PacketsSent
	packetsReceived := finalStats.PacketsReceived - initialStats.PacketsReceived

	metrics.BytesSent = bytesSent
	metrics.BytesReceived = bytesReceived
	metrics.PacketsSent = packetsSent
	metrics.PacketsReceived = packetsReceived
	metrics.UploadSpeed = float64(bytesSent) / (1024 * 1024) / duration
	metrics.DownloadSpeed = float64(bytesReceived) / (1024 * 1024) / duration
	metrics.Errin = finalStats.Errin - initialStats.Errin
	metrics.Errout = finalStats.Errout - initialStats.Errout
	metrics.Dropin = finalStats.Dropin - initialStats.Dropin
	metrics.Dropout = finalStats.Dropout - initialStats.Dropout
	metrics.Fifoin = finalStats.Fifoin - initialStats.Fifoin
	metrics.Fifoout = finalStats.Fifoout - initialStats.Fifoout

	latency, err := measureLatency()
	if err == nil {
		metrics.Latency = latency
	}

	return metrics, nil
}

func getNetworkStats() (networkStats, error) {
	var stats networkStats

	counters, err := gopsutilnet.IOCounters(false)
	if err != nil {
		return stats, err
	}

	for _, counter := range counters {
		stats.BytesSent += counter.BytesSent
		stats.BytesReceived += counter.BytesRecv
		stats.PacketsSent += counter.PacketsSent
		stats.PacketsReceived += counter.PacketsRecv
		stats.Errin += counter.Errin
		stats.Errout += counter.Errout
		stats.Dropin += counter.Dropin
		stats.Dropout += counter.Dropout
		stats.Fifoin += counter.Fifoin
		stats.Fifoout += counter.Fifoout
	}

	return stats, nil
}

func measureLatency() (time.Duration, error) {

	start := time.Now()
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 2*time.Second)
	duration := time.Since(start)

	if err != nil {

		start = time.Now()
		conn, err = net.DialTimeout("udp", "8.8.8.8:53", 2*time.Second)
		duration = time.Since(start)
		if err != nil {
			return 0, err
		}
	}

	if conn != nil {
		conn.Close()
	}

	return duration, nil
}
