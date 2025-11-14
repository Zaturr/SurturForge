package disk

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type DiskMetrics struct {
	Duration      time.Time
	BytesRead     int64
	BytesWritten  int64
	Operations    int64
	ThroughputMBs float64
	IOPS          float64
	Latency       time.Duration
	DiskUsage     float64
}

type ioStats struct {
	BytesRead    int64
	BytesWritten int64
	Operations   int64
}

func GetDiskMetrics() (DiskMetrics, error) {
	metrics := DiskMetrics{
		Duration: time.Now(),
	}

	// Paso 1: Obtener métricas iniciales
	initialIO, err := getIOStats()
	if err != nil {
		return metrics, err
	}

	// Paso 2: Esperar un período de medición (1 segundo)
	time.Sleep(1 * time.Second)

	// Paso 3: Obtener métricas finales
	finalIO, err := getIOStats()
	if err != nil {
		return metrics, err
	}

	// Paso 4: Calcular diferencias
	duration := 1.0 // segundos
	bytesRead := finalIO.BytesRead - initialIO.BytesRead
	bytesWritten := finalIO.BytesWritten - initialIO.BytesWritten
	operations := finalIO.Operations - initialIO.Operations

	metrics.BytesRead = bytesRead
	metrics.BytesWritten = bytesWritten
	metrics.Operations = operations
	metrics.ThroughputMBs = float64(bytesRead+bytesWritten) / (1024 * 1024) / duration
	metrics.IOPS = float64(operations) / duration

	if operations > 0 {
		metrics.Latency = time.Duration(float64(duration)/float64(operations)*1000) * time.Millisecond
	}

	// Obtener uso del disco
	usage, err := getDiskUsage()
	if err == nil {
		metrics.DiskUsage = usage
	}

	return metrics, nil
}

func getIOStats() (ioStats, error) {
	var stats ioStats

	if runtime.GOOS == "windows" {
		// Windows: usar PowerShell para obtener contadores acumulativos de disco
		// Usamos contadores que acumulan valores desde el inicio del sistema
		cmd := exec.Command("powershell", "-Command",
			"$counters = @('\\PhysicalDisk(_Total)\\Disk Read Bytes','\\PhysicalDisk(_Total)\\Disk Write Bytes','\\PhysicalDisk(_Total)\\Disk Reads','\\PhysicalDisk(_Total)\\Disk Writes'); "+
				"$result = Get-Counter -Counter $counters -ErrorAction SilentlyContinue; "+
				"if ($result) { $result.CounterSamples | ForEach-Object { $_.RawValue } }")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			// Filtrar líneas vacías
			var values []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					values = append(values, line)
				}
			}
			if len(values) >= 4 {
				readBytes, _ := strconv.ParseInt(values[0], 10, 64)
				writeBytes, _ := strconv.ParseInt(values[1], 10, 64)
				reads, _ := strconv.ParseInt(values[2], 10, 64)
				writes, _ := strconv.ParseInt(values[3], 10, 64)

				stats.BytesRead = readBytes
				stats.BytesWritten = writeBytes
				stats.Operations = reads + writes
			}
		}
	} else {
		// Linux: leer /proc/diskstats
		data, err := os.ReadFile("/proc/diskstats")
		if err != nil {
			return stats, err
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 14 {
				// Formato: major minor name rio rmerge rsect ruse wio wmerge wsect wuse running aveq
				// fields[0]=major, fields[1]=minor, fields[2]=name
				// fields[3]=reads, fields[5]=sectors_read, fields[7]=writes, fields[9]=sectors_written

				reads, _ := strconv.ParseInt(fields[3], 10, 64)
				sectorsRead, _ := strconv.ParseInt(fields[5], 10, 64)
				writes, _ := strconv.ParseInt(fields[7], 10, 64)
				sectorsWritten, _ := strconv.ParseInt(fields[9], 10, 64)

				// 1 sector = 512 bytes típicamente
				stats.BytesRead += sectorsRead * 512
				stats.BytesWritten += sectorsWritten * 512
				stats.Operations += reads + writes
			}
		}
	}

	return stats, nil
}

func getDiskUsage() (float64, error) {
	if runtime.GOOS == "windows" {
		// Windows: usar wmic para obtener uso del disco
		cmd := exec.Command("wmic", "logicaldisk", "where", "DeviceID='C:'", "get", "Size,FreeSpace", "/format:list")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		var size, freeSpace int64
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Size=") {
				val, _ := strconv.ParseInt(strings.TrimPrefix(line, "Size="), 10, 64)
				size = val
			}
			if strings.HasPrefix(line, "FreeSpace=") {
				val, _ := strconv.ParseInt(strings.TrimPrefix(line, "FreeSpace="), 10, 64)
				freeSpace = val
			}
		}

		if size > 0 {
			used := float64(size - freeSpace)
			usagePercent := (used / float64(size)) * 100.0
			return usagePercent, nil
		}
	} else {
		// Linux: usar df
		cmd := exec.Command("df", "-h", "/")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				// fields[4] contiene el porcentaje de uso (ej: "45%")
				usageStr := strings.TrimSuffix(fields[4], "%")
				usage, err := strconv.ParseFloat(usageStr, 64)
				if err == nil {
					return usage, nil
				}
			}
		}
	}

	return 0, nil
}

func GetDiskInfo() (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("wmic", "diskdrive", "get", "size,model,serialnumber", "/format:list")
	} else {
		cmd = exec.Command("lsblk", "-o", "NAME,SIZE,MODEL,SERIAL")
	}
	diskInfo, err := cmd.Output()
	if err != nil {
		return "", err
	}
	info := string(diskInfo)
	if runtime.GOOS == "windows" {
		info = formatWmicDiskInfo(info)
	}
	return info, nil
}

func formatWmicDiskInfo(info string) string {
	return info
}
