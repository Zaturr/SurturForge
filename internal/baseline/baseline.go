package baseline

import (
	"fmt"
	stdnet "net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type HardwareInfo struct {
	CPUCoresPhysical  int
	CPUCoresLogical   int
	RAMTotal          uint64
	DiskTotal         uint64
	DiskFree          uint64
	NetworkInterfaces []NetworkInterfaceInfo
	Timestamp         time.Time
}

type NetworkInterfaceInfo struct {
	Name         string
	MTU          int
	Speed        uint64
	HardwareAddr string
	IPAddresses  []string
}

type SystemBaseline struct {
	CPUIdlePercent  float64
	MemoryFree      uint64
	MemoryAvailable uint64
	DiskFree        uint64
	NetworkLatency  time.Duration
	Timestamp       time.Time
}

type EnvironmentConfig struct {
	OS                  string
	Architecture        string
	GoVersion           string
	ProcessCount        int
	HighCPUProcesses    []ProcessInfo
	HighMemoryProcesses []ProcessInfo
	EnvironmentVars     map[string]string
	Recommendations     []string
	Timestamp           time.Time
}

type ProcessInfo struct {
	PID        int32
	Name       string
	CPUPercent float64
	MemoryMB   float64
}

type BaselineResult struct {
	Hardware    HardwareInfo
	Baseline    SystemBaseline
	Environment EnvironmentConfig
}

func DetectHardware() (HardwareInfo, error) {
	info := HardwareInfo{
		Timestamp: time.Now(),
	}

	physicalCores, err := cpu.Counts(false)
	if err == nil {
		info.CPUCoresPhysical = physicalCores
	}

	logicalCores, err := cpu.Counts(true)
	if err == nil {
		info.CPUCoresLogical = logicalCores
	}

	vmem, err := mem.VirtualMemory()
	if err == nil {
		info.RAMTotal = vmem.Total
	}

	partitions, err := disk.Partitions(false)
	if err == nil {
		var totalDisk uint64
		var freeDisk uint64
		for _, partition := range partitions {
			usage, err := disk.Usage(partition.Mountpoint)
			if err == nil {
				totalDisk += usage.Total
				freeDisk += usage.Free
			}
		}
		info.DiskTotal = totalDisk
		info.DiskFree = freeDisk
	}

	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if strings.Contains(strings.ToLower(iface.Name), "loopback") {
				continue
			}

			netInfo := NetworkInterfaceInfo{
				Name:         iface.Name,
				MTU:          int(iface.MTU),
				HardwareAddr: iface.HardwareAddr,
			}

			for _, addr := range iface.Addrs {
				netInfo.IPAddresses = append(netInfo.IPAddresses, addr.Addr)
			}

			netInfo.Speed = 0
			info.NetworkInterfaces = append(info.NetworkInterfaces, netInfo)
		}
	}

	return info, nil
}

func EstablishBaseline() (SystemBaseline, error) {
	baseline := SystemBaseline{
		Timestamp: time.Now(),
	}

	time.Sleep(500 * time.Millisecond)

	var cpuSamples []float64
	for i := 0; i < 3; i++ {
		percentages, err := cpu.Percent(500*time.Millisecond, false)
		if err == nil && len(percentages) > 0 {
			cpuSamples = append(cpuSamples, 100.0-percentages[0])
		}
		time.Sleep(200 * time.Millisecond)
	}

	if len(cpuSamples) > 0 {
		var sum float64
		for _, sample := range cpuSamples {
			sum += sample
		}
		baseline.CPUIdlePercent = sum / float64(len(cpuSamples))
	}

	vmem, err := mem.VirtualMemory()
	if err == nil {
		baseline.MemoryFree = vmem.Free
		baseline.MemoryAvailable = vmem.Available
	}

	partitions, err := disk.Partitions(false)
	if err == nil {
		var totalFree uint64
		for _, partition := range partitions {
			usage, err := disk.Usage(partition.Mountpoint)
			if err == nil {
				totalFree += usage.Free
			}
		}
		baseline.DiskFree = totalFree
	}

	var latencies []time.Duration
	for i := 0; i < 5; i++ {
		latency, err := measureNetworkLatency()
		if err == nil {
			latencies = append(latencies, latency)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(latencies) > 0 {
		var total time.Duration
		for _, lat := range latencies {
			total += lat
		}
		baseline.NetworkLatency = total / time.Duration(len(latencies))
	}

	return baseline, nil
}

func measureNetworkLatency() (time.Duration, error) {
	servers := []string{"8.8.8.8:53", "1.1.1.1:53", "208.67.222.222:53"}

	for _, server := range servers {
		start := time.Now()
		conn, err := stdnet.DialTimeout("tcp", server, 2*time.Second)
		duration := time.Since(start)

		if err == nil {
			if conn != nil {
				conn.Close()
			}
			return duration, nil
		}
	}

	return 0, fmt.Errorf("no se pudo medir la latencia de red")
}

func ConfigureEnvironment() (EnvironmentConfig, error) {
	config := EnvironmentConfig{
		Timestamp:       time.Now(),
		EnvironmentVars: make(map[string]string),
		Recommendations: []string{},
	}

	config.OS = runtime.GOOS
	config.Architecture = runtime.GOARCH
	config.GoVersion = runtime.Version()

	processes, err := process.Processes()
	if err == nil {
		config.ProcessCount = len(processes)
	}

	var highCPU []ProcessInfo
	var highMemory []ProcessInfo

	for _, proc := range processes {
		if len(highCPU)+len(highMemory) > 100 {
			break
		}

		name, _ := proc.Name()
		cpuPercent, _ := proc.CPUPercent()
		memInfo, _ := proc.MemoryInfo()

		if cpuPercent > 5.0 {
			highCPU = append(highCPU, ProcessInfo{
				PID:        proc.Pid,
				Name:       name,
				CPUPercent: cpuPercent,
			})
		}

		if memInfo != nil && memInfo.RSS > 100*1024*1024 {
			highMemory = append(highMemory, ProcessInfo{
				PID:      proc.Pid,
				Name:     name,
				MemoryMB: float64(memInfo.RSS) / (1024 * 1024),
			})
		}
	}

	config.HighCPUProcesses = highCPU
	config.HighMemoryProcesses = highMemory

	envVars := []string{
		"GOMAXPROCS", "GOGC", "GODEBUG",
		"PATH", "TEMP", "TMP",
		"NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE",
	}

	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			config.EnvironmentVars[envVar] = value
		}
	}

	config.Recommendations = generateRecommendations(config)

	return config, nil
}

func generateRecommendations(config EnvironmentConfig) []string {
	var recommendations []string

	if _, exists := config.EnvironmentVars["GOMAXPROCS"]; !exists {
		recommendations = append(recommendations,
			"Considerar establecer GOMAXPROCS para controlar el número de CPUs utilizadas")
	}

	if len(config.HighCPUProcesses) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de CPU. Considerar cerrarlos antes del benchmark.",
				len(config.HighCPUProcesses)))
	}

	if len(config.HighMemoryProcesses) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Se detectaron %d procesos con alto uso de memoria. Considerar cerrarlos antes del benchmark.",
				len(config.HighMemoryProcesses)))
	}

	if config.OS == "windows" {
		recommendations = append(recommendations,
			"En Windows, considerar desactivar servicios innecesarios y actualizaciones automáticas")
		recommendations = append(recommendations,
			"Ejecutar el benchmark como administrador puede mejorar la precisión de las mediciones")
	}

	if config.ProcessCount > 200 {
		recommendations = append(recommendations,
			fmt.Sprintf("Alto número de procesos (%d). Esto puede afectar el rendimiento del benchmark.",
				config.ProcessCount))
	}

	return recommendations
}

func RunBaseline() (BaselineResult, error) {
	result := BaselineResult{}

	hardware, err := DetectHardware()
	if err != nil {
		return result, fmt.Errorf("error en detección de hardware: %w", err)
	}
	result.Hardware = hardware

	baseline, err := EstablishBaseline()
	if err != nil {
		return result, fmt.Errorf("error estableciendo baseline: %w", err)
	}
	result.Baseline = baseline

	env, err := ConfigureEnvironment()
	if err != nil {
		return result, fmt.Errorf("error configurando entorno: %w", err)
	}
	result.Environment = env

	return result, nil
}

func FormatBytes(bytes uint64) string {
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

func PrintBaseline(result BaselineResult) {
	fmt.Println("=== BASELINE DEL SISTEMA ===")
	fmt.Println()

	fmt.Println("--- DETECCIÓN DE HARDWARE ---")
	fmt.Printf("Cores Físicos: %d\n", result.Hardware.CPUCoresPhysical)
	fmt.Printf("Cores Lógicos: %d\n", result.Hardware.CPUCoresLogical)
	fmt.Printf("RAM Total: %s\n", FormatBytes(result.Hardware.RAMTotal))
	fmt.Printf("Disco Total: %s\n", FormatBytes(result.Hardware.DiskTotal))
	fmt.Printf("Disco Libre: %s\n", FormatBytes(result.Hardware.DiskFree))
	fmt.Printf("Interfaces de Red: %d\n", len(result.Hardware.NetworkInterfaces))
	for _, iface := range result.Hardware.NetworkInterfaces {
		fmt.Printf("  - %s (MTU: %d)", iface.Name, iface.MTU)
		if len(iface.IPAddresses) > 0 {
			fmt.Printf(" IPs: %s", strings.Join(iface.IPAddresses, ", "))
		}
		fmt.Println()
	}
	fmt.Println()

	fmt.Println("--- BASELINE DEL SISTEMA ---")
	fmt.Printf("CPU Idle: %.2f%%\n", result.Baseline.CPUIdlePercent)
	fmt.Printf("Memoria Libre: %s\n", FormatBytes(result.Baseline.MemoryFree))
	fmt.Printf("Memoria Disponible: %s\n", FormatBytes(result.Baseline.MemoryAvailable))
	fmt.Printf("Disco Libre: %s\n", FormatBytes(result.Baseline.DiskFree))
	fmt.Printf("Latencia de Red Base: %v\n", result.Baseline.NetworkLatency)
	fmt.Println()

	fmt.Println("--- CONFIGURACIÓN DEL ENTORNO ---")
	fmt.Printf("Sistema Operativo: %s\n", result.Environment.OS)
	fmt.Printf("Arquitectura: %s\n", result.Environment.Architecture)
	fmt.Printf("Versión de Go: %s\n", result.Environment.GoVersion)
	fmt.Printf("Número de Procesos: %d\n", result.Environment.ProcessCount)

	if len(result.Environment.HighCPUProcesses) > 0 {
		fmt.Printf("\nProcesos con Alto Uso de CPU (>5%%):\n")
		for _, proc := range result.Environment.HighCPUProcesses[:min(10, len(result.Environment.HighCPUProcesses))] {
			fmt.Printf("  - %s (PID: %d, CPU: %.2f%%)\n", proc.Name, proc.PID, proc.CPUPercent)
		}
	}

	if len(result.Environment.HighMemoryProcesses) > 0 {
		fmt.Printf("\nProcesos con Alto Uso de Memoria (>100MB):\n")
		for _, proc := range result.Environment.HighMemoryProcesses[:min(10, len(result.Environment.HighMemoryProcesses))] {
			fmt.Printf("  - %s (PID: %d, Memoria: %.2f MB)\n", proc.Name, proc.PID, proc.MemoryMB)
		}
	}

	if len(result.Environment.EnvironmentVars) > 0 {
		fmt.Printf("\nVariables de Entorno Relevantes:\n")
		for key, value := range result.Environment.EnvironmentVars {
			fmt.Printf("  - %s = %s\n", key, value)
		}
	}

	if len(result.Environment.Recommendations) > 0 {
		fmt.Printf("\nRecomendaciones:\n")
		for i, rec := range result.Environment.Recommendations {
			fmt.Printf("  %d. %s\n", i+1, rec)
		}
	}
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func SetEnvironmentVariables() error {
	if os.Getenv("GOMAXPROCS") == "" {
		cores, err := cpu.Counts(true)
		if err == nil {
			os.Setenv("GOMAXPROCS", fmt.Sprintf("%d", cores))
		}
	}

	os.Setenv("GOGC", "100")

	return nil
}

func CheckAdminRights() bool {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("net", "session")
		err := cmd.Run()
		return err == nil
	}
	return os.Geteuid() == 0
}
