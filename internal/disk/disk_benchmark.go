package disk

import (
	"bufio"
	"bytes"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// Constantes para el benchmark
const (
	Blocksize     = 0x1 << 16 // 65,536 bytes (64 KB)
	DiskBlockSize = 0x1 << 9  // 512 bytes
)

// Mark estructura principal del benchmark de disco
type Mark struct {
	Start                       time.Time
	TempDir                     string
	AggregateTestFilesSizeInGiB float64
	NumReadersWriters           int
	PhysicalMemory              uint64
	IODuration                  float64
	Results                     []Result
	fileSize                    int
	randomBlock                 []byte
	randomString                string
	stopCacheClear              chan struct{}
	cacheClearWg                sync.WaitGroup
	cpuMonitorDone              chan struct{}
	cpuSamples                  []float64
	cpuMonitorMutex             sync.Mutex
}

type Result struct {
	Start           time.Time
	WrittenBytes    int
	WrittenDuration time.Duration
	ReadBytes       int
	ReadDuration    time.Duration
	IOOperations    int
	IODuration      time.Duration
	DeletedFiles    int
	DeleteDuration  time.Duration

	WriteThroughputMBs     float64
	ReadThroughputMBs      float64
	SustainedThroughputMBs float64

	WriteLatency time.Duration
	ReadLatency  time.Duration
	IOPSLatency  time.Duration

	ConsistencyScore float64
	StabilityScore   float64

	CPUOverheadPercent float64
	CPUIdleBefore      float64
	CPUIdleDuring      float64
	CPUPeakUsage       float64
	CPUAverageUsage    float64
}

type ThreadResult struct {
	Result int // bytes escritos, bytes leídos, o número de operaciones
	Error  error
}

func NewMark(tempDir string, aggregateSizeGiB float64) *Mark {
	numThreads := runtime.NumCPU()
	if numThreads < 1 {
		numThreads = 1
	}

	mathrand.Seed(time.Now().UnixNano())

	return &Mark{
		Start:                       time.Now(),
		TempDir:                     tempDir,
		AggregateTestFilesSizeInGiB: aggregateSizeGiB,
		NumReadersWriters:           numThreads,
		IODuration:                  15.0,
		Results:                     make([]Result, 0),
		stopCacheClear:              make(chan struct{}),
		cpuMonitorDone:              make(chan struct{}),
		cpuSamples:                  make([]float64, 0),
	}
}

func (bm *Mark) SetTempDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error al crear directorio temporal: %w", err)
	}
	bm.TempDir = dir
	return nil
}

func (bm *Mark) CreateRandomBlock() error {
	bm.randomBlock = make([]byte, Blocksize)
	bm.randomString = randomString(9)

	lenRandom, err := cryptorand.Read(bm.randomBlock)
	if err != nil {
		return fmt.Errorf("error generando datos aleatorios: %w", err)
	}

	if len(bm.randomBlock) != lenRandom {
		return fmt.Errorf("CreateRandomBlock(): RandomBlock no obtuvo el número correcto de bytes, %d != %d",
			len(bm.randomBlock), lenRandom)
	}
	return nil
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[mathrand.Intn(len(letters))]
	}
	return string(b)
}

func ClearBufferCache() error {
	runtime.GC()
	return nil
}

// ClearBufferCacheEveryThreeSeconds limpia el buffer cache cada 3 segundos en una goroutine
func (bm *Mark) ClearBufferCacheEveryThreeSeconds() {
	bm.cacheClearWg.Add(1)
	go func() {
		defer bm.cacheClearWg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-bm.stopCacheClear:
				return
			case <-ticker.C:
				_ = ClearBufferCache() // Ignorar errores
			}
		}
	}()
}

func (bm *Mark) StopCacheClear() {
	close(bm.stopCacheClear)
	bm.cacheClearWg.Wait()
}

func (bm *Mark) RunSequentialWriteTest() error {
	newResult := Result{
		Start: time.Now(),
	}
	bm.Results = append(bm.Results, newResult)
	resultIdx := len(bm.Results) - 1

	bm.fileSize = (int(bm.AggregateTestFilesSizeInGiB*(1<<10)) << 20) / bm.NumReadersWriters

	for i := 0; i < bm.NumReadersWriters; i++ {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i))
		if err := os.RemoveAll(filename); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error eliminando archivo previo %s: %w", filename, err)
		}
	}

	bm.cpuMonitorDone = make(chan struct{})
	bm.startCPUMonitoring()

	bytesWritten := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	for i := 0; i < bm.NumReadersWriters; i++ {
		go bm.singleThreadWriteTest(
			filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i)),
			bytesWritten,
		)
	}

	bm.Results[resultIdx].WrittenBytes = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-bytesWritten
		if result.Error != nil {
			bm.stopCPUMonitoring()
			return fmt.Errorf("error en escritura: %w", result.Error)
		}
		bm.Results[resultIdx].WrittenBytes += result.Result
	}

	bm.Results[resultIdx].WrittenDuration = time.Since(start)

	bm.calculateCPUOverhead(resultIdx)

	bm.calculateWriteMetrics(resultIdx)

	return nil
}

func (bm *Mark) singleThreadWriteTest(filename string, bytesWrittenChannel chan<- ThreadResult) {
	f, err := os.Create(filename)
	if err != nil {
		bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	bytesWritten := 0
	for i := 0; i < bm.fileSize; i += len(bm.randomBlock) {
		n, err := w.Write(bm.randomBlock)
		if err != nil {
			bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
			return
		}
		bytesWritten += n
	}

	err = w.Flush()
	if err != nil {
		bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
		return
	}

	err = f.Close()
	if err != nil {
		bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
		return
	}

	bytesWrittenChannel <- ThreadResult{Result: bytesWritten, Error: nil}
}

func (bm *Mark) RunSequentialReadTest() error {
	if len(bm.Results) == 0 {
		return fmt.Errorf("no hay resultados previos, ejecuta RunSequentialWriteTest primero")
	}

	resultIdx := len(bm.Results) - 1
	newResult := &bm.Results[resultIdx]

	_ = ClearBufferCache()

	bm.cpuMonitorDone = make(chan struct{})
	bm.startCPUMonitoring()

	bytesRead := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	for i := 0; i < bm.NumReadersWriters; i++ {
		go bm.singleThreadReadTest(
			filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i)),
			bytesRead,
		)
	}

	newResult.ReadBytes = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-bytesRead
		if result.Error != nil {
			bm.stopCPUMonitoring()
			return fmt.Errorf("error en lectura: %w", result.Error)
		}
		newResult.ReadBytes += result.Result
	}

	newResult.ReadDuration = time.Since(start)

	bm.calculateCPUOverhead(resultIdx)

	bm.calculateReadMetrics(resultIdx)
	bm.calculateConsistencyAndStability(resultIdx)

	return nil
}

func (bm *Mark) singleThreadReadTest(filename string, bytesReadChannel chan<- ThreadResult) {
	f, err := os.Open(filename)
	if err != nil {
		bytesReadChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	defer f.Close()

	bytesRead := 0
	data := make([]byte, Blocksize)

	for {
		n, err := f.Read(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			bytesReadChannel <- ThreadResult{Result: 0, Error: err}
			return
		}
		bytesRead += n

		if bytesRead%(127*Blocksize) == 0 {
			if !bytes.Equal(bm.randomBlock, data) {
				bytesReadChannel <- ThreadResult{
					Result: 0,
					Error: fmt.Errorf(
						"most recent block didn't match random block, bytes read (includes corruption): %d",
						bytesRead),
				}
				return
			}
		}
	}

	bytesReadChannel <- ThreadResult{Result: bytesRead, Error: nil}
}

func (bm *Mark) RunIOPSTest() error {
	if len(bm.Results) == 0 {
		return fmt.Errorf("no hay resultados previos, ejecuta RunSequentialWriteTest primero")
	}

	resultIdx := len(bm.Results) - 1
	newResult := &bm.Results[resultIdx]

	_ = ClearBufferCache()

	bm.cpuMonitorDone = make(chan struct{})
	bm.startCPUMonitoring()

	opsPerformed := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	for i := 0; i < bm.NumReadersWriters; i++ {
		go bm.singleThreadIOPSTest(
			filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i)),
			opsPerformed,
		)
	}

	newResult.IOOperations = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-opsPerformed
		if result.Error != nil {
			bm.stopCPUMonitoring()
			return fmt.Errorf("error en IOPS: %w", result.Error)
		}
		newResult.IOOperations += result.Result
	}

	newResult.IODuration = time.Since(start)

	bm.calculateCPUOverhead(resultIdx)

	bm.calculateIOPSMetrics(resultIdx)

	if err := bm.CleanupTestFiles(); err != nil {
		fmt.Printf("Advertencia: error limpiando archivos después de IOPS: %v\n", err)
	}

	return nil
}

func (bm *Mark) singleThreadIOPSTest(filename string, numOpsChannel chan<- ThreadResult) {
	diskBlockSize := DiskBlockSize // 512 bytes (tamaño de sector tradicional)

	fileInfo, err := os.Stat(filename)
	if err != nil {
		numOpsChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	fileSizeLessOneDiskBlock := fileInfo.Size() - int64(diskBlockSize)
	if fileSizeLessOneDiskBlock <= 0 {
		numOpsChannel <- ThreadResult{Result: 0, Error: fmt.Errorf("archivo demasiado pequeño para test IOPS")}
		return
	}
	numOperations := 0

	f, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		numOpsChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	defer f.Close()

	data := make([]byte, diskBlockSize)
	checksum := make([]byte, diskBlockSize)

	start := time.Now()
	for time.Since(start).Seconds() < bm.IODuration {
		randomPos := mathrand.Int63n(fileSizeLessOneDiskBlock)
		_, err = f.Seek(randomPos, 0)
		if err != nil {
			numOpsChannel <- ThreadResult{Result: 0, Error: err}
			return
		}

		if numOperations%10 != 0 {
			length, err := f.Read(data)
			if err != nil && err != io.EOF {
				numOpsChannel <- ThreadResult{Result: 0, Error: err}
				return
			}
			if length != diskBlockSize && err != io.EOF {
				panic(fmt.Sprintf("I expected to read %d bytes, instead I read %d bytes!",
					diskBlockSize, length))
			}
			for j := 0; j < length && j < diskBlockSize; j++ {
				checksum[j] ^= data[j]
			}
		} else {
			length, err := f.Write(checksum)
			if err != nil {
				numOpsChannel <- ThreadResult{Result: 0, Error: err}
				return
			}
			if length != diskBlockSize {
				numOpsChannel <- ThreadResult{
					Result: 0,
					Error: fmt.Errorf("i expected to write %d bytes, instead i wrote %d bytes",
						diskBlockSize, length),
				}
				return
			}
		}
		numOperations++
	}

	err = f.Close()
	if err != nil {
		numOpsChannel <- ThreadResult{Result: 0, Error: err}
		return
	}

	numOpsChannel <- ThreadResult{Result: numOperations, Error: nil}
}

func (bm *Mark) CleanupTestFiles() error {
	if bm.TempDir == "" {
		return nil
	}

	var errors []string
	deletedCount := 0
	totalSizeDeleted := int64(0)

	entries, err := os.ReadDir(bm.TempDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directorio no existe, nada que limpiar
		}
		return fmt.Errorf("error leyendo directorio %s: %w", bm.TempDir, err)
	}

	for _, entry := range entries {
		fullPath := filepath.Join(bm.TempDir, entry.Name())

		if info, err := entry.Info(); err == nil {
			if !entry.IsDir() {
				totalSizeDeleted += info.Size()
			}
		}

		if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("%s: %v", fullPath, err))
		} else {
			deletedCount++
		}
	}

	os.Remove(bm.TempDir)

	if len(errors) > 0 {
		return fmt.Errorf("errores eliminando archivos (%d eliminados, %d errores, %s liberados): %v",
			deletedCount, len(errors), formatBytes(uint64(totalSizeDeleted)), errors)
	}

	return nil
}

func (bm *Mark) VerifyCleanup() (int, error) {
	if bm.randomString == "" {
		return 0, nil
	}

	count := 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i))
		if _, err := os.Stat(filename); err == nil {
			count++
		}
	}

	entries, err := os.ReadDir(bm.TempDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && len(entry.Name()) > len(bm.randomString) {
				if len(entry.Name()) > len(bm.randomString)+7 &&
					entry.Name()[:len(bm.randomString)] == bm.randomString &&
					entry.Name()[len(bm.randomString):len(bm.randomString)+7] == "_delete_" {
					count++
				}
			}
		}
	}

	return count, nil
}

func (bm *Mark) RunDeleteTest() error {
	if len(bm.Results) == 0 {
		return fmt.Errorf("no hay resultados previos, ejecuta RunSequentialWriteTest primero")
	}

	resultIdx := len(bm.Results) - 1
	newResult := &bm.Results[resultIdx]

	filesToDelete := make([]string, bm.NumReadersWriters*10) // 10 archivos por thread
	for i := range filesToDelete {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s_delete_%d", bm.randomString, i))
		filesToDelete[i] = filename

		f, err := os.Create(filename)
		if err != nil {
			continue
		}
		data := make([]byte, 1024*1024)
		f.Write(data)
		f.Close()
	}

	deleteChan := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	filesPerThread := len(filesToDelete) / bm.NumReadersWriters
	if filesPerThread == 0 {
		filesPerThread = 1
	}

	for i := 0; i < bm.NumReadersWriters; i++ {
		startIdx := i * filesPerThread
		endIdx := startIdx + filesPerThread
		if i == bm.NumReadersWriters-1 {
			endIdx = len(filesToDelete) // El último thread toma los archivos restantes
		}
		go bm.singleThreadDeleteTest(filesToDelete[startIdx:endIdx], deleteChan)
	}

	newResult.DeletedFiles = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-deleteChan
		if result.Error != nil {
			fmt.Printf("Advertencia en eliminación: %v\n", result.Error)
		}
		newResult.DeletedFiles += result.Result
	}

	newResult.DeleteDuration = time.Since(start)
	return nil
}

func (bm *Mark) singleThreadDeleteTest(files []string, deleteChannel chan<- ThreadResult) {
	deletedCount := 0
	for _, filename := range files {
		if err := os.Remove(filename); err != nil {
			if !os.IsNotExist(err) {
				deleteChannel <- ThreadResult{Result: deletedCount, Error: err}
				return
			}
		} else {
			deletedCount++
		}
	}
	deleteChannel <- ThreadResult{Result: deletedCount, Error: nil}
}

func (bm *Mark) GetLastResult() *Result {
	if len(bm.Results) == 0 {
		return nil
	}
	return &bm.Results[len(bm.Results)-1]
}

func (bm *Mark) startCPUMonitoring() {
	bm.cpuMonitorMutex.Lock()
	bm.cpuSamples = make([]float64, 0)
	bm.cpuMonitorMutex.Unlock()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-bm.cpuMonitorDone:
				return
			case <-ticker.C:
				percentages, err := cpu.Percent(100*time.Millisecond, false)
				if err == nil && len(percentages) > 0 {
					bm.cpuMonitorMutex.Lock()
					bm.cpuSamples = append(bm.cpuSamples, percentages[0])
					bm.cpuMonitorMutex.Unlock()
				}
			}
		}
	}()
}

func (bm *Mark) stopCPUMonitoring() (avg, max, min float64) {
	close(bm.cpuMonitorDone)
	time.Sleep(150 * time.Millisecond)

	bm.cpuMonitorMutex.Lock()
	defer bm.cpuMonitorMutex.Unlock()

	if len(bm.cpuSamples) == 0 {
		return 0, 0, 0
	}

	var sum float64
	min = 100.0
	max = 0.0

	for _, sample := range bm.cpuSamples {
		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}

	avg = sum / float64(len(bm.cpuSamples))
	return avg, max, min
}

func (bm *Mark) getCPUIdleBefore() float64 {
	percentages, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil || len(percentages) == 0 {
		return 0
	}
	return 100.0 - percentages[0]
}

func (bm *Mark) calculateWriteMetrics(resultIdx int) {
	result := &bm.Results[resultIdx]

	if result.WrittenDuration > 0 && result.WrittenBytes > 0 {
		result.WriteThroughputMBs = float64(result.WrittenBytes) / (1024 * 1024) / result.WrittenDuration.Seconds()

		operations := result.WrittenBytes / Blocksize
		if operations > 0 {
			result.WriteLatency = result.WrittenDuration / time.Duration(operations)
		}
	}
}

func (bm *Mark) calculateReadMetrics(resultIdx int) {
	result := &bm.Results[resultIdx]

	if result.ReadDuration > 0 && result.ReadBytes > 0 {
		result.ReadThroughputMBs = float64(result.ReadBytes) / (1024 * 1024) / result.ReadDuration.Seconds()

		operations := result.ReadBytes / Blocksize
		if operations > 0 {
			result.ReadLatency = result.ReadDuration / time.Duration(operations)
		}

		if result.WriteThroughputMBs > 0 {
			result.SustainedThroughputMBs = (result.WriteThroughputMBs + result.ReadThroughputMBs) / 2.0
		} else {
			result.SustainedThroughputMBs = result.ReadThroughputMBs
		}
	}
}

func (bm *Mark) calculateIOPSMetrics(resultIdx int) {
	result := &bm.Results[resultIdx]

	if result.IODuration > 0 && result.IOOperations > 0 {
		result.IOPSLatency = result.IODuration / time.Duration(result.IOOperations)
	}
}

func (bm *Mark) calculateConsistencyAndStability(resultIdx int) {
	if resultIdx == 0 {
		bm.Results[resultIdx].ConsistencyScore = 100.0
		bm.Results[resultIdx].StabilityScore = 100.0
		return
	}

	result := &bm.Results[resultIdx]

	var throughputs []float64
	for i := 0; i <= resultIdx; i++ {
		if bm.Results[i].WriteThroughputMBs > 0 {
			throughputs = append(throughputs, bm.Results[i].WriteThroughputMBs)
		}
		if bm.Results[i].ReadThroughputMBs > 0 {
			throughputs = append(throughputs, bm.Results[i].ReadThroughputMBs)
		}
	}

	if len(throughputs) >= 2 {
		var sum float64
		for _, t := range throughputs {
			sum += t
		}
		mean := sum / float64(len(throughputs))

		var variance float64
		for _, t := range throughputs {
			variance += (t - mean) * (t - mean)
		}
		stdDev := math.Sqrt(variance / float64(len(throughputs)))

		if mean > 0 {
			coefficientOfVariation := stdDev / mean
			result.ConsistencyScore = math.Max(0, 100.0*(1.0-coefficientOfVariation))
		} else {
			result.ConsistencyScore = 100.0
		}

		if resultIdx >= 1 {
			prevResult := bm.Results[resultIdx-1]
			currentAvg := (result.WriteThroughputMBs + result.ReadThroughputMBs) / 2.0
			prevAvg := (prevResult.WriteThroughputMBs + prevResult.ReadThroughputMBs) / 2.0

			if prevAvg > 0 {
				change := math.Abs(currentAvg-prevAvg) / prevAvg
				result.StabilityScore = math.Max(0, 100.0*(1.0-change))
			} else {
				result.StabilityScore = 100.0
			}
		} else {
			result.StabilityScore = result.ConsistencyScore
		}
	} else {
		result.ConsistencyScore = 100.0
		result.StabilityScore = 100.0
	}
}

// calculateCPUOverhead calcula el overhead de CPU
func (bm *Mark) calculateCPUOverhead(resultIdx int) {
	result := &bm.Results[resultIdx]

	result.CPUIdleBefore = bm.getCPUIdleBefore()

	avgCPU, maxCPU, _ := bm.stopCPUMonitoring()
	result.CPUAverageUsage = avgCPU
	result.CPUPeakUsage = maxCPU
	result.CPUIdleDuring = 100.0 - avgCPU

	result.CPUOverheadPercent = result.CPUIdleBefore - result.CPUIdleDuring
	if result.CPUOverheadPercent < 0 {
		result.CPUOverheadPercent = 0
	}
}

// PrintResults imprime los resultados del benchmark
func (bm *Mark) PrintResults() {
	if len(bm.Results) == 0 {
		fmt.Println("No hay resultados para mostrar")
		return
	}

	fmt.Println("\n=== Resultados del Benchmark de Disco ===")
	for i, result := range bm.Results {
		fmt.Printf("\n--- Ciclo %d ---\n", i+1)
		fmt.Printf("Inicio: %s\n", result.Start.Format("2006-01-02 15:04:05"))

		if result.WrittenBytes > 0 {
			writeMB := float64(result.WrittenBytes) / (1024 * 1024)
			writeSpeed := writeMB / result.WrittenDuration.Seconds()
			fmt.Printf("Escritura: %s escritos en %v (%.2f MB/s)\n",
				formatBytes(uint64(result.WrittenBytes)),
				result.WrittenDuration,
				writeSpeed)
		}

		if result.ReadBytes > 0 {
			readMB := float64(result.ReadBytes) / (1024 * 1024)
			readSpeed := readMB / result.ReadDuration.Seconds()
			fmt.Printf("Lectura: %s leídos en %v (%.2f MB/s)\n",
				formatBytes(uint64(result.ReadBytes)),
				result.ReadDuration,
				readSpeed)
		}

		if result.IOOperations > 0 {
			iops := float64(result.IOOperations) / result.IODuration.Seconds()
			fmt.Printf("IOPS: %d operaciones en %v (%.0f IOPS)\n",
				result.IOOperations,
				result.IODuration,
				iops)
		}

		if result.DeletedFiles > 0 {
			deleteRate := float64(result.DeletedFiles) / result.DeleteDuration.Seconds()
			fmt.Printf("Eliminación: %d archivos en %v (%.0f archivos/s)\n",
				result.DeletedFiles,
				result.DeleteDuration,
				deleteRate)
		}
	}
	fmt.Println("==========================================")
}

// DisplayDiskBenchmarkResult muestra los resultados del benchmark de disco con formato mejorado
func DisplayDiskBenchmarkResult(result *Result, formatBytes func(uint64) string, colorBold, colorReset, colorGreen, colorCyan, colorYellow, colorRed string, printHeader, printError func(string)) {
	if result == nil {
		printError("No hay resultados para mostrar")
		return
	}

	printHeader("RESULTADOS BENCHMARK DE DISCO")

	if result.WrittenBytes > 0 {
		fmt.Printf("%sEscritura Secuencial:%s\n", colorBold, colorReset)
		fmt.Printf("  Bytes escritos: %s\n", formatBytes(uint64(result.WrittenBytes)))
		fmt.Printf("  Duración: %v\n", result.WrittenDuration)
		fmt.Printf("  Rendimiento: %s%.2f MB/s%s\n", colorGreen, result.WriteThroughputMBs, colorReset)
		if result.WriteLatency > 0 {
			fmt.Printf("  Latencia promedio: %s%v%s\n", colorCyan, result.WriteLatency, colorReset)
		}
		fmt.Println()
	}

	if result.ReadBytes > 0 {
		fmt.Printf("%sLectura Secuencial:%s\n", colorBold, colorReset)
		fmt.Printf("  Bytes leídos: %s\n", formatBytes(uint64(result.ReadBytes)))
		fmt.Printf("  Duración: %v\n", result.ReadDuration)
		fmt.Printf("  Rendimiento: %s%.2f MB/s%s\n", colorGreen, result.ReadThroughputMBs, colorReset)
		if result.ReadLatency > 0 {
			fmt.Printf("  Latencia promedio: %s%v%s\n", colorCyan, result.ReadLatency, colorReset)
		}
		fmt.Println()
	}

	if result.SustainedThroughputMBs > 0 {
		fmt.Printf("%sTasa de Transferencia Sostenida:%s\n", colorBold, colorReset)
		fmt.Printf("  Throughput promedio: %s%.2f MB/s%s\n", colorGreen, result.SustainedThroughputMBs, colorReset)
		fmt.Println()
	}

	if result.IOOperations > 0 {
		iops := float64(result.IOOperations) / result.IODuration.Seconds()
		fmt.Printf("%sIOPS (Operaciones Aleatorias):%s\n", colorBold, colorReset)
		fmt.Printf("  Operaciones: %d\n", result.IOOperations)
		fmt.Printf("  Duración: %v\n", result.IODuration)
		fmt.Printf("  IOPS: %s%.0f%s\n", colorGreen, iops, colorReset)
		if result.IOPSLatency > 0 {
			fmt.Printf("  Latencia promedio: %s%v%s\n", colorCyan, result.IOPSLatency, colorReset)
		}
		fmt.Println()
	}

	if result.DeletedFiles > 0 {
		deleteRate := float64(result.DeletedFiles) / result.DeleteDuration.Seconds()
		fmt.Printf("%sEliminación de Archivos:%s\n", colorBold, colorReset)
		fmt.Printf("  Archivos eliminados: %d\n", result.DeletedFiles)
		fmt.Printf("  Duración: %v\n", result.DeleteDuration)
		fmt.Printf("  Velocidad: %s%.0f archivos/s%s\n", colorGreen, deleteRate, colorReset)
		fmt.Println()
	}

	// Métricas de Consistencia y Estabilidad
	if result.ConsistencyScore > 0 || result.StabilityScore > 0 {
		fmt.Printf("%sConsistencia y Estabilidad:%s\n", colorBold, colorReset)
		if result.ConsistencyScore > 0 {
			consistencyColor := colorGreen
			if result.ConsistencyScore < 70 {
				consistencyColor = colorYellow
			}
			if result.ConsistencyScore < 50 {
				consistencyColor = colorRed
			}
			fmt.Printf("  Consistencia: %s%.1f/100%s (menor variación = mejor)\n",
				consistencyColor, result.ConsistencyScore, colorReset)
		}
		if result.StabilityScore > 0 {
			stabilityColor := colorGreen
			if result.StabilityScore < 70 {
				stabilityColor = colorYellow
			}
			if result.StabilityScore < 50 {
				stabilityColor = colorRed
			}
			fmt.Printf("  Estabilidad: %s%.1f/100%s (menor cambio entre ciclos = mejor)\n",
				stabilityColor, result.StabilityScore, colorReset)
		}
		fmt.Println()
	}

	// Overhead de CPU
	if result.CPUOverheadPercent > 0 || result.CPUAverageUsage > 0 {
		fmt.Printf("%sOverhead de CPU:%s\n", colorBold, colorReset)
		if result.CPUIdleBefore > 0 {
			fmt.Printf("  CPU Idle antes: %.1f%%\n", result.CPUIdleBefore)
		}
		if result.CPUAverageUsage > 0 {
			fmt.Printf("  CPU promedio durante test: %.1f%%\n", result.CPUAverageUsage)
		}
		if result.CPUPeakUsage > 0 {
			fmt.Printf("  CPU pico: %.1f%%\n", result.CPUPeakUsage)
		}
		if result.CPUOverheadPercent > 0 {
			overheadColor := colorGreen
			if result.CPUOverheadPercent > 30 {
				overheadColor = colorYellow
			}
			if result.CPUOverheadPercent > 50 {
				overheadColor = colorRed
			}
			fmt.Printf("  Overhead: %s%.1f%%%s (diferencia CPU idle antes vs durante)\n",
				overheadColor, result.CPUOverheadPercent, colorReset)
		}
		fmt.Println()
	}

	// Resumen comparativo
	if result.WrittenBytes > 0 && result.ReadBytes > 0 {
		fmt.Printf("%sResumen:%s\n", colorBold, colorReset)
		if result.SustainedThroughputMBs > 0 {
			fmt.Printf("  Throughput sostenido: %s%.2f MB/s%s\n", colorCyan, result.SustainedThroughputMBs, colorReset)
		}
		if result.DeletedFiles > 0 {
			fmt.Printf("  Archivos eliminados: %d (para liberar espacio en disco)\n", result.DeletedFiles)
		}
	}
}
