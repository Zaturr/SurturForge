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

// HardwareInfo contiene información del hardware detectado
type HardwareInfo struct {
	CPUCoresPhysical  int                    // Número de CPUs/núcleos físicos
	CPUCoresLogical   int                    // Número de cores lógicos
	RAMTotal          uint64                 // RAM total disponible en bytes
	DiskTotal         uint64                 // Espacio en disco total en bytes
	DiskFree          uint64                 // Espacio en disco libre en bytes
	NetworkInterfaces []NetworkInterfaceInfo // Información de interfaces de red
	Timestamp         time.Time
}

// NetworkInterfaceInfo contiene información de una interfaz de red
type NetworkInterfaceInfo struct {
	Name         string
	MTU          int
	Speed        uint64 // Velocidad en bits por segundo (si está disponible)
	HardwareAddr string
	IPAddresses  []string
}

// SystemBaseline contiene el estado inicial del sistema
type SystemBaseline struct {
	CPUIdlePercent  float64       // Estado inicial de CPU (idle)
	MemoryFree      uint64        // Memoria libre inicial en bytes
	MemoryAvailable uint64        // Memoria disponible inicial en bytes
	DiskFree        uint64        // Espacio en disco libre en bytes
	NetworkLatency  time.Duration // Latencia de red base
	Timestamp       time.Time
}

// EnvironmentConfig contiene la configuración del entorno
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

// ProcessInfo contiene información de un proceso
type ProcessInfo struct {
	PID        int32
	Name       string
	CPUPercent float64
	MemoryMB   float64
}

// BaselineResult contiene toda la información del baseline
type BaselineResult struct {
	Hardware    HardwareInfo
	Baseline    SystemBaseline
	Environment EnvironmentConfig
}

// DetectHardware detecta y retorna información del hardware del sistema
func DetectHardware() (HardwareInfo, error) {
	info := HardwareInfo{
		Timestamp: time.Now(),
	}

	// Detectar CPUs/núcleos
	physicalCores, err := cpu.Counts(false)
	if err == nil {
		info.CPUCoresPhysical = physicalCores
	}

	logicalCores, err := cpu.Counts(true)
	if err == nil {
		info.CPUCoresLogical = logicalCores
	}

	// Detectar RAM total
	vmem, err := mem.VirtualMemory()
	if err == nil {
		info.RAMTotal = vmem.Total
	}

	// Detectar espacio en disco
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

	// Detectar capacidad de red
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			// Filtrar interfaces loopback y no activas
			if strings.Contains(strings.ToLower(iface.Name), "loopback") {
				continue
			}

			netInfo := NetworkInterfaceInfo{
				Name:         iface.Name,
				MTU:          int(iface.MTU),
				HardwareAddr: iface.HardwareAddr,
			}

			// Obtener direcciones IP
			for _, addr := range iface.Addrs {
				netInfo.IPAddresses = append(netInfo.IPAddresses, addr.Addr)
			}

			// Intentar obtener velocidad de la interfaz (puede no estar disponible)
			// En Windows, esto puede requerir WMI
			netInfo.Speed = 0 // Se deja en 0 si no se puede obtener

			info.NetworkInterfaces = append(info.NetworkInterfaces, netInfo)
		}
	}

	return info, nil
}

// EstablishBaseline establece el baseline del sistema en estado idle
func EstablishBaseline() (SystemBaseline, error) {
	baseline := SystemBaseline{
		Timestamp: time.Now(),
	}

	// Esperar un momento para que el sistema se estabilice
	time.Sleep(500 * time.Millisecond)

	// Medir CPU idle (promedio de múltiples muestras)
	var cpuSamples []float64
	for i := 0; i < 3; i++ {
		percentages, err := cpu.Percent(500*time.Millisecond, false)
		if err == nil && len(percentages) > 0 {
			// CPU idle = 100 - uso
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

	// Memoria libre inicial
	vmem, err := mem.VirtualMemory()
	if err == nil {
		baseline.MemoryFree = vmem.Free
		baseline.MemoryAvailable = vmem.Available
	}

	// Espacio en disco libre
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

	// Latencia de red base (promedio de múltiples mediciones)
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

// measureNetworkLatency mide la latencia de red a un servidor externo
func measureNetworkLatency() (time.Duration, error) {
	// Intentar con múltiples servidores DNS conocidos
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

// ConfigureEnvironment analiza y configura el entorno para el benchmark
func ConfigureEnvironment() (EnvironmentConfig, error) {
	config := EnvironmentConfig{
		Timestamp:       time.Now(),
		EnvironmentVars: make(map[string]string),
		Recommendations: []string{},
	}

	// Información del sistema
	config.OS = runtime.GOOS
	config.Architecture = runtime.GOARCH
	config.GoVersion = runtime.Version()

	// Contar procesos
	processes, err := process.Processes()
	if err == nil {
		config.ProcessCount = len(processes)
	}

	// Identificar procesos con alto uso de CPU y memoria
	var highCPU []ProcessInfo
	var highMemory []ProcessInfo

	for _, proc := range processes {
		// Limitar el análisis a los primeros 100 procesos para no sobrecargar
		if len(highCPU)+len(highMemory) > 100 {
			break
		}

		name, _ := proc.Name()
		cpuPercent, _ := proc.CPUPercent()
		memInfo, _ := proc.MemoryInfo()

		if cpuPercent > 5.0 { // Procesos usando más del 5% de CPU
			highCPU = append(highCPU, ProcessInfo{
				PID:        proc.Pid,
				Name:       name,
				CPUPercent: cpuPercent,
			})
		}

		if memInfo != nil && memInfo.RSS > 100*1024*1024 { // Más de 100MB
			highMemory = append(highMemory, ProcessInfo{
				PID:      proc.Pid,
				Name:     name,
				MemoryMB: float64(memInfo.RSS) / (1024 * 1024),
			})
		}
	}

	config.HighCPUProcesses = highCPU
	config.HighMemoryProcesses = highMemory

	// Variables de entorno relevantes
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

	// Generar recomendaciones
	config.Recommendations = generateRecommendations(config)

	return config, nil
}

// generateRecommendations genera recomendaciones basadas en el estado del sistema
func generateRecommendations(config EnvironmentConfig) []string {
	var recommendations []string

	// Recomendación sobre GOMAXPROCS
	if _, exists := config.EnvironmentVars["GOMAXPROCS"]; !exists {
		recommendations = append(recommendations,
			"Considerar establecer GOMAXPROCS para controlar el número de CPUs utilizadas")
	}

	// Recomendación sobre procesos de alto consumo
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

	// Recomendación sobre Windows
	if config.OS == "windows" {
		recommendations = append(recommendations,
			"En Windows, considerar desactivar servicios innecesarios y actualizaciones automáticas")
		recommendations = append(recommendations,
			"Ejecutar el benchmark como administrador puede mejorar la precisión de las mediciones")
	}

	// Recomendación sobre número de procesos
	if config.ProcessCount > 200 {
		recommendations = append(recommendations,
			fmt.Sprintf("Alto número de procesos (%d). Esto puede afectar el rendimiento del benchmark.",
				config.ProcessCount))
	}

	return recommendations
}

// RunBaseline ejecuta el baseline completo y retorna todos los resultados
func RunBaseline() (BaselineResult, error) {
	result := BaselineResult{}

	// 1. Detección de hardware
	hardware, err := DetectHardware()
	if err != nil {
		return result, fmt.Errorf("error en detección de hardware: %w", err)
	}
	result.Hardware = hardware

	// 2. Baseline del sistema
	baseline, err := EstablishBaseline()
	if err != nil {
		return result, fmt.Errorf("error estableciendo baseline: %w", err)
	}
	result.Baseline = baseline

	// 3. Configuración del entorno
	env, err := ConfigureEnvironment()
	if err != nil {
		return result, fmt.Errorf("error configurando entorno: %w", err)
	}
	result.Environment = env

	return result, nil
}

// FormatBytes formatea bytes a formato legible
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

// PrintBaseline imprime el resultado del baseline de forma legible
func PrintBaseline(result BaselineResult) {
	fmt.Println("=== BASELINE DEL SISTEMA ===")
	fmt.Println()

	// Hardware
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

	// Baseline
	fmt.Println("--- BASELINE DEL SISTEMA ---")
	fmt.Printf("CPU Idle: %.2f%%\n", result.Baseline.CPUIdlePercent)
	fmt.Printf("Memoria Libre: %s\n", FormatBytes(result.Baseline.MemoryFree))
	fmt.Printf("Memoria Disponible: %s\n", FormatBytes(result.Baseline.MemoryAvailable))
	fmt.Printf("Disco Libre: %s\n", FormatBytes(result.Baseline.DiskFree))
	fmt.Printf("Latencia de Red Base: %v\n", result.Baseline.NetworkLatency)
	fmt.Println()

	// Entorno
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

// min retorna el mínimo de dos enteros
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetEnvironmentVariables establece variables de entorno para el benchmark
func SetEnvironmentVariables() error {
	// Establecer GOMAXPROCS si no está configurado
	if os.Getenv("GOMAXPROCS") == "" {
		cores, err := cpu.Counts(true)
		if err == nil {
			os.Setenv("GOMAXPROCS", fmt.Sprintf("%d", cores))
		}
	}

	// Establecer otras variables de entorno útiles para benchmarks
	os.Setenv("GOGC", "100") // Garbage collector estándar

	return nil
}

// CheckAdminRights verifica si el programa se ejecuta con privilegios de administrador
func CheckAdminRights() bool {
	if runtime.GOOS == "windows" {
		// En Windows, intentar ejecutar un comando que requiere admin
		cmd := exec.Command("net", "session")
		err := cmd.Run()
		return err == nil
	}
	// En Unix/Linux, verificar si el UID es 0
	return os.Geteuid() == 0
}
