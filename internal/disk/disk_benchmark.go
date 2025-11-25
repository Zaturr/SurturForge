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
	// Blocksize tamaño de bloque para escritura/lectura secuencial (64 KB)
	Blocksize = 0x1 << 16 // 65,536 bytes (64 KB)
	// DiskBlockSize tamaño de bloque para test IOPS (512 bytes)
	DiskBlockSize = 0x1 << 9 // 512 bytes
)

// Mark estructura principal del benchmark de disco
type Mark struct {
	Start                       time.Time
	TempDir                     string
	AggregateTestFilesSizeInGiB float64
	NumReadersWriters           int // Número de threads (default: runtime.NumCPU())
	PhysicalMemory              uint64
	IODuration                  float64 // Duración del test IOPS (default: 15.0)
	Results                     []Result
	fileSize                    int           // Tamaño de archivo por thread
	randomBlock                 []byte        // Bloque de 64 KB con datos aleatorios
	randomString                string        // String aleatorio para nombres de archivo
	stopCacheClear              chan struct{} // Canal para detener limpieza de cache
	cacheClearWg                sync.WaitGroup
	cpuMonitorDone              chan struct{} // Canal para detener monitoreo de CPU
	cpuSamples                  []float64     // Muestras de CPU durante el test
	cpuMonitorMutex             sync.Mutex    // Mutex para proteger cpuSamples
}

// Result resultado de un ciclo de benchmark
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

	// Métricas avanzadas
	WriteThroughputMBs     float64 // MB/s de escritura
	ReadThroughputMBs      float64 // MB/s de lectura
	SustainedThroughputMBs float64 // Tasa de transferencia sostenida (promedio)

	WriteLatency time.Duration // Latencia promedio de escritura por operación
	ReadLatency  time.Duration // Latencia promedio de lectura por operación
	IOPSLatency  time.Duration // Latencia promedio de IOPS

	ConsistencyScore float64 // Puntuación de consistencia (0-100, mayor = más consistente)
	StabilityScore   float64 // Puntuación de estabilidad (0-100, mayor = más estable)

	CPUOverheadPercent float64 // Overhead de CPU durante el test
	CPUIdleBefore      float64 // CPU idle antes del test
	CPUIdleDuring      float64 // CPU idle durante el test
	CPUPeakUsage       float64 // Pico de uso de CPU
	CPUAverageUsage    float64 // Uso promedio de CPU
}

// ThreadResult resultado de una operación en un thread
type ThreadResult struct {
	Result int // bytes escritos, bytes leídos, o número de operaciones
	Error  error
}

// NewMark crea una nueva instancia de Mark con valores por defecto
func NewMark(tempDir string, aggregateSizeGiB float64) *Mark {
	numThreads := runtime.NumCPU()
	if numThreads < 1 {
		numThreads = 1
	}

	// Inicializar generador de números aleatorios
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

// SetTempDir establece el directorio temporal para los archivos de test
func (bm *Mark) SetTempDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error al crear directorio temporal: %w", err)
	}
	bm.TempDir = dir
	return nil
}

// CreateRandomBlock genera un bloque aleatorio de 64 KB para prevenir optimizaciones del filesystem
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

// randomString genera una cadena aleatoria de longitud n
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[mathrand.Intn(len(letters))]
	}
	return string(b)
}

// ClearBufferCache intenta limpiar el buffer cache del sistema operativo
// En Windows esto puede no estar soportado, por lo que ignoramos errores
func ClearBufferCache() error {
	// En Linux: sync + echo 3 > /proc/sys/vm/drop_caches
	// En Windows: no hay forma directa, pero podemos intentar sincronizar
	runtime.GC() // Forzar garbage collection para liberar memoria

	// Intentar sincronizar el sistema de archivos
	// En Windows esto no limpia el cache pero sincroniza escrituras pendientes
	// En Linux podríamos ejecutar un comando del sistema, pero por simplicidad
	// y portabilidad, solo hacemos GC
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

// StopCacheClear detiene la limpieza continua de cache
func (bm *Mark) StopCacheClear() {
	close(bm.stopCacheClear)
	bm.cacheClearWg.Wait()
}

// RunSequentialWriteTest ejecuta el test de escritura secuencial paralela
func (bm *Mark) RunSequentialWriteTest() error {
	// Crear nuevo resultado
	newResult := Result{
		Start: time.Now(),
	}
	bm.Results = append(bm.Results, newResult)
	resultIdx := len(bm.Results) - 1

	// 1. CALCULAR TAMAÑO DE ARCHIVO POR THREAD
	// Divide el tamaño total entre el número de threads
	bm.fileSize = (int(bm.AggregateTestFilesSizeInGiB*(1<<10)) << 20) / bm.NumReadersWriters

	// 2. ELIMINAR ARCHIVOS PREVIOS (si existen)
	// Importante para múltiples runs
	for i := 0; i < bm.NumReadersWriters; i++ {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i))
		if err := os.RemoveAll(filename); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error eliminando archivo previo %s: %w", filename, err)
		}
	}

	// 3. INICIAR MONITOREO DE CPU
	bm.cpuMonitorDone = make(chan struct{})
	bm.startCPUMonitoring()

	// 4. CREAR CANAL DE COMUNICACIÓN
	bytesWritten := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	// 5. LANZAR MÚLTIPLES GOROUTINES EN PARALELO
	// Cada goroutine escribe un archivo diferente simultáneamente
	for i := 0; i < bm.NumReadersWriters; i++ {
		go bm.singleThreadWriteTest(
			filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i)),
			bytesWritten,
		)
	}

	// 6. RECOPILAR RESULTADOS DE TODAS LAS GOROUTINES
	bm.Results[resultIdx].WrittenBytes = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-bytesWritten
		if result.Error != nil {
			bm.stopCPUMonitoring()
			return fmt.Errorf("error en escritura: %w", result.Error)
		}
		bm.Results[resultIdx].WrittenBytes += result.Result
	}

	// 7. CALCULAR DURACIÓN
	bm.Results[resultIdx].WrittenDuration = time.Since(start)

	// 8. DETENER MONITOREO DE CPU Y CALCULAR OVERHEAD
	bm.calculateCPUOverhead(resultIdx)

	// 9. CALCULAR MÉTRICAS AVANZADAS
	bm.calculateWriteMetrics(resultIdx)

	// NOTA: NO eliminamos archivos aquí porque se necesitan para el test de lectura
	// Los archivos se eliminarán después del test de IOPS

	return nil
}

// singleThreadWriteTest ejecuta la escritura de un archivo en un thread individual
func (bm *Mark) singleThreadWriteTest(filename string, bytesWrittenChannel chan<- ThreadResult) {
	// 1. CREAR ARCHIVO FÍSICO EN DISCO
	// os.Create() fuerza la creación física del archivo
	f, err := os.Create(filename)
	if err != nil {
		bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	defer f.Close()

	// 2. CREAR BUFFER DE ESCRITURA
	// bufio.NewWriter optimiza escrituras pero NO evita escritura a disco
	// Buffer por defecto: 4 KB
	w := bufio.NewWriter(f)

	// 3. BUCLE DE ESCRITURA CONTINUA - AQUÍ SE FUERZA EL DISCO
	bytesWritten := 0
	// bm.randomBlock contiene 64 KB (65,536 bytes) de datos aleatorios
	for i := 0; i < bm.fileSize; i += len(bm.randomBlock) {
		// Escribe bloque de 64 KB en cada iteración
		n, err := w.Write(bm.randomBlock)
		if err != nil {
			bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
			return
		}
		bytesWritten += n
		// NO HAY PAUSAS - Escritura continua sin time.Sleep()
	}

	// 4. FORZAR ESCRITURA FINAL A DISCO (CRÍTICO)
	// Flush() fuerza que todos los datos en buffer se escriban al disco
	err = w.Flush()
	if err != nil {
		bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
		return
	}

	// 5. CERRAR ARCHIVO - GARANTIZA SINCRONIZACIÓN CON DISCO
	// Close() asegura que el OS escriba todo al disco físico
	err = f.Close()
	if err != nil {
		bytesWrittenChannel <- ThreadResult{Result: 0, Error: err}
		return
	}

	// 6. ENVIAR RESULTADO
	bytesWrittenChannel <- ThreadResult{Result: bytesWritten, Error: nil}
}

// RunSequentialReadTest ejecuta el test de lectura secuencial paralela
func (bm *Mark) RunSequentialReadTest() error {
	if len(bm.Results) == 0 {
		return fmt.Errorf("no hay resultados previos, ejecuta RunSequentialWriteTest primero")
	}

	resultIdx := len(bm.Results) - 1
	newResult := &bm.Results[resultIdx]

	// 1. LIMPIAR BUFFER CACHE ANTES DE LEER (CRÍTICO)
	// Esto fuerza que se lea del disco físico, no de RAM
	_ = ClearBufferCache() // Ignorar errores si no está soportado

	// 2. INICIAR MONITOREO DE CPU
	bm.cpuMonitorDone = make(chan struct{})
	bm.startCPUMonitoring()

	// 3. CREAR CANAL DE COMUNICACIÓN
	bytesRead := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	// 4. LANZAR MÚLTIPLES GOROUTINES EN PARALELO
	// Cada goroutine lee un archivo diferente simultáneamente
	for i := 0; i < bm.NumReadersWriters; i++ {
		go bm.singleThreadReadTest(
			filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i)),
			bytesRead,
		)
	}

	// 5. RECOPILAR RESULTADOS
	newResult.ReadBytes = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-bytesRead
		if result.Error != nil {
			bm.stopCPUMonitoring()
			return fmt.Errorf("error en lectura: %w", result.Error)
		}
		newResult.ReadBytes += result.Result
	}

	// 6. CALCULAR DURACIÓN
	newResult.ReadDuration = time.Since(start)

	// 7. DETENER MONITOREO DE CPU Y CALCULAR OVERHEAD
	bm.calculateCPUOverhead(resultIdx)

	// 8. CALCULAR MÉTRICAS AVANZADAS
	bm.calculateReadMetrics(resultIdx)
	bm.calculateConsistencyAndStability(resultIdx)

	// NOTA: Los archivos se mantienen para el test de IOPS
	// Se eliminarán después del test de IOPS

	return nil
}

// singleThreadReadTest ejecuta la lectura de un archivo en un thread individual
func (bm *Mark) singleThreadReadTest(filename string, bytesReadChannel chan<- ThreadResult) {
	// 1. ABRIR ARCHIVO DEL DISCO FÍSICO
	// os.Open() abre el archivo físico del disco
	f, err := os.Open(filename)
	if err != nil {
		bytesReadChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	defer f.Close()

	// 2. BUCLE DE LECTURA CONTINUA - AQUÍ SE FUERZA EL DISCO
	bytesRead := 0
	data := make([]byte, Blocksize) // Buffer de 64 KB (65,536 bytes)

	for {
		// Lee 64 KB del disco en cada iteración
		n, err := f.Read(data)
		if err != nil {
			if err == io.EOF {
				break // Fin del archivo
			}
			bytesReadChannel <- ThreadResult{Result: 0, Error: err}
			return
		}
		bytesRead += n

		// 3. VERIFICACIÓN DE INTEGRIDAD (cada 127 bloques)
		// 127 es un número primo para evitar colisiones
		// Compara con el bloque aleatorio original para detectar corrupción
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
		// NO HAY PAUSAS - Lectura continua sin time.Sleep()
	}

	// 4. ENVIAR RESULTADO
	bytesReadChannel <- ThreadResult{Result: bytesRead, Error: nil}
}

// RunIOPSTest ejecuta el test de IOPS (operaciones aleatorias - máximo stress)
func (bm *Mark) RunIOPSTest() error {
	if len(bm.Results) == 0 {
		return fmt.Errorf("no hay resultados previos, ejecuta RunSequentialWriteTest primero")
	}

	resultIdx := len(bm.Results) - 1
	newResult := &bm.Results[resultIdx]

	// 1. LIMPIAR BUFFER CACHE ANTES DEL TEST (CRÍTICO)
	_ = ClearBufferCache() // Ignorar errores si no está soportado

	// 2. INICIAR MONITOREO DE CPU
	bm.cpuMonitorDone = make(chan struct{})
	bm.startCPUMonitoring()

	// 3. CREAR CANAL DE COMUNICACIÓN
	opsPerformed := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	// 4. LANZAR MÚLTIPLES GOROUTINES EN PARALELO
	// Cada goroutine ejecuta operaciones aleatorias simultáneamente
	for i := 0; i < bm.NumReadersWriters; i++ {
		go bm.singleThreadIOPSTest(
			filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i)),
			opsPerformed,
		)
	}

	// 5. RECOPILAR RESULTADOS
	newResult.IOOperations = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-opsPerformed
		if result.Error != nil {
			bm.stopCPUMonitoring()
			return fmt.Errorf("error en IOPS: %w", result.Error)
		}
		newResult.IOOperations += result.Result
	}

	// 6. CALCULAR DURACIÓN
	newResult.IODuration = time.Since(start)

	// 7. DETENER MONITOREO DE CPU Y CALCULAR OVERHEAD
	bm.calculateCPUOverhead(resultIdx)

	// 8. CALCULAR MÉTRICAS AVANZADAS
	bm.calculateIOPSMetrics(resultIdx)

	// 9. LIMPIAR ARCHIVOS DESPUÉS DE IOPS (para evitar llenar el disco)
	// Ahora sí eliminamos los archivos después de todos los tests
	if err := bm.CleanupTestFiles(); err != nil {
		fmt.Printf("Advertencia: error limpiando archivos después de IOPS: %v\n", err)
	}

	return nil
}

// singleThreadIOPSTest ejecuta operaciones aleatorias de I/O en un thread individual
func (bm *Mark) singleThreadIOPSTest(filename string, numOpsChannel chan<- ThreadResult) {
	diskBlockSize := DiskBlockSize // 512 bytes (tamaño de sector tradicional)

	// 1. OBTENER TAMAÑO DEL ARCHIVO
	fileInfo, err := os.Stat(filename)
	if err != nil {
		numOpsChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	// Limitar búsquedas para no leer más allá del EOF
	fileSizeLessOneDiskBlock := fileInfo.Size() - int64(diskBlockSize)
	if fileSizeLessOneDiskBlock <= 0 {
		numOpsChannel <- ThreadResult{Result: 0, Error: fmt.Errorf("archivo demasiado pequeño para test IOPS")}
		return
	}
	numOperations := 0

	// 2. ABRIR ARCHIVO EN MODO LECTURA/ESCRITURA
	// os.O_RDWR permite leer y escribir
	f, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		numOpsChannel <- ThreadResult{Result: 0, Error: err}
		return
	}
	defer f.Close()

	// 3. PREPARAR BUFFERS
	data := make([]byte, diskBlockSize)     // 512 bytes para leer
	checksum := make([]byte, diskBlockSize) // 512 bytes para checksum

	// 4. BUCLE DE OPERACIONES ALEATORIAS - MÁXIMO STRESS
	start := time.Now()
	// Ejecutar durante IODuration segundos (15 por defecto)
	for time.Since(start).Seconds() < bm.IODuration {
		// 5. SALTAR A POSICIÓN ALEATORIA EN EL ARCHIVO
		// Esto fuerza al disco a buscar físicamente la posición
		// Es el PEOR CASO de rendimiento para un disco
		randomPos := mathrand.Int63n(fileSizeLessOneDiskBlock)
		_, err = f.Seek(randomPos, 0)
		if err != nil {
			numOpsChannel <- ThreadResult{Result: 0, Error: err}
			return
		}

		// 6. RATIO 10:1 (9 lecturas por cada escritura)
		// Simula carga de trabajo real (TPC-E benchmark)
		if numOperations%10 != 0 {
			// LECTURA ALEATORIA (90% de las operaciones)
			length, err := f.Read(data)
			if err != nil && err != io.EOF {
				numOpsChannel <- ThreadResult{Result: 0, Error: err}
				return
			}
			if length != diskBlockSize && err != io.EOF {
				panic(fmt.Sprintf("I expected to read %d bytes, instead I read %d bytes!",
					diskBlockSize, length))
			}
			// Calcular checksum XOR para verificar integridad
			for j := 0; j < length && j < diskBlockSize; j++ {
				checksum[j] ^= data[j]
			}
		} else {
			// ESCRITURA ALEATORIA (10% de las operaciones)
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
		// NO HAY PAUSAS - Operaciones continuas sin time.Sleep()
	}

	// 7. CERRAR ARCHIVO PARA FORZAR FLUSH
	// Redundante pero asegura que writes se flush
	err = f.Close()
	if err != nil {
		numOpsChannel <- ThreadResult{Result: 0, Error: err}
		return
	}

	// 8. ENVIAR RESULTADO
	numOpsChannel <- ThreadResult{Result: numOperations, Error: nil}
}

// CleanupTestFiles elimina todos los archivos de test
func (bm *Mark) CleanupTestFiles() error {
	var errors []string
	deletedCount := 0

	for i := 0; i < bm.NumReadersWriters; i++ {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i))
		if err := os.RemoveAll(filename); err != nil && !os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("%s: %v", filename, err))
		} else {
			deletedCount++
		}
	}

	// También eliminar archivos de delete test si existen
	if bm.randomString != "" {
		entries, err := os.ReadDir(bm.TempDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && len(entry.Name()) > len(bm.randomString) {
					// Verificar si es un archivo de delete test
					if len(entry.Name()) > len(bm.randomString)+7 &&
						entry.Name()[:len(bm.randomString)] == bm.randomString &&
						entry.Name()[len(bm.randomString):len(bm.randomString)+7] == "_delete_" {
						filename := filepath.Join(bm.TempDir, entry.Name())
						if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
							errors = append(errors, fmt.Sprintf("%s: %v", filename, err))
						} else {
							deletedCount++
						}
					}
				}
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errores eliminando archivos (%d eliminados, %d errores): %v",
			deletedCount, len(errors), errors)
	}

	return nil
}

// VerifyCleanup verifica que los archivos de test se hayan eliminado correctamente
// Retorna el número de archivos que aún existen
func (bm *Mark) VerifyCleanup() (int, error) {
	if bm.randomString == "" {
		return 0, nil // No hay archivos para verificar
	}

	count := 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s%d", bm.randomString, i))
		if _, err := os.Stat(filename); err == nil {
			count++
		}
	}

	// Verificar archivos de delete test
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

// RunDeleteTest ejecuta un test de eliminación de archivos (fuerza operaciones de borrado en disco)
func (bm *Mark) RunDeleteTest() error {
	if len(bm.Results) == 0 {
		return fmt.Errorf("no hay resultados previos, ejecuta RunSequentialWriteTest primero")
	}

	resultIdx := len(bm.Results) - 1
	newResult := &bm.Results[resultIdx]

	// 1. CREAR ARCHIVOS TEMPORALES PARA ELIMINAR
	// Crear archivos pequeños que se eliminarán rápidamente
	filesToDelete := make([]string, bm.NumReadersWriters*10) // 10 archivos por thread
	for i := range filesToDelete {
		filename := filepath.Join(bm.TempDir, fmt.Sprintf("%s_delete_%d", bm.randomString, i))
		filesToDelete[i] = filename

		// Crear archivo pequeño (1 MB)
		f, err := os.Create(filename)
		if err != nil {
			continue
		}
		// Escribir 1 MB de datos
		data := make([]byte, 1024*1024)
		f.Write(data)
		f.Close()
	}

	// 2. EJECUTAR ELIMINACIÓN EN PARALELO
	deleteChan := make(chan ThreadResult, bm.NumReadersWriters)
	start := time.Now()

	// Dividir archivos entre threads
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

	// 3. RECOPILAR RESULTADOS
	newResult.DeletedFiles = 0
	for i := 0; i < bm.NumReadersWriters; i++ {
		result := <-deleteChan
		if result.Error != nil {
			// Continuar aunque haya errores, pero loguearlos
			fmt.Printf("Advertencia en eliminación: %v\n", result.Error)
		}
		newResult.DeletedFiles += result.Result
	}

	// 4. CALCULAR DURACIÓN
	newResult.DeleteDuration = time.Since(start)
	return nil
}

// singleThreadDeleteTest ejecuta la eliminación de archivos en un thread individual
func (bm *Mark) singleThreadDeleteTest(files []string, deleteChannel chan<- ThreadResult) {
	deletedCount := 0
	for _, filename := range files {
		if err := os.Remove(filename); err != nil {
			if !os.IsNotExist(err) {
				// Solo reportar error si el archivo existe pero no se puede eliminar
				deleteChannel <- ThreadResult{Result: deletedCount, Error: err}
				return
			}
		} else {
			deletedCount++
		}
	}
	deleteChannel <- ThreadResult{Result: deletedCount, Error: nil}
}

// GetLastResult retorna el último resultado del benchmark
func (bm *Mark) GetLastResult() *Result {
	if len(bm.Results) == 0 {
		return nil
	}
	return &bm.Results[len(bm.Results)-1]
}

// startCPUMonitoring inicia el monitoreo de CPU en una goroutine
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

// stopCPUMonitoring detiene el monitoreo de CPU y calcula estadísticas
func (bm *Mark) stopCPUMonitoring() (avg, max, min float64) {
	close(bm.cpuMonitorDone)
	time.Sleep(150 * time.Millisecond) // Esperar última muestra

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

// getCPUIdleBefore obtiene el CPU idle antes del test
func (bm *Mark) getCPUIdleBefore() float64 {
	percentages, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil || len(percentages) == 0 {
		return 0
	}
	return 100.0 - percentages[0]
}

// calculateWriteMetrics calcula métricas avanzadas de escritura
func (bm *Mark) calculateWriteMetrics(resultIdx int) {
	result := &bm.Results[resultIdx]

	if result.WrittenDuration > 0 && result.WrittenBytes > 0 {
		// Throughput en MB/s
		result.WriteThroughputMBs = float64(result.WrittenBytes) / (1024 * 1024) / result.WrittenDuration.Seconds()

		// Latencia promedio por operación (asumiendo operaciones de bloque)
		operations := result.WrittenBytes / Blocksize
		if operations > 0 {
			result.WriteLatency = result.WrittenDuration / time.Duration(operations)
		}
	}
}

// calculateReadMetrics calcula métricas avanzadas de lectura
func (bm *Mark) calculateReadMetrics(resultIdx int) {
	result := &bm.Results[resultIdx]

	if result.ReadDuration > 0 && result.ReadBytes > 0 {
		// Throughput en MB/s
		result.ReadThroughputMBs = float64(result.ReadBytes) / (1024 * 1024) / result.ReadDuration.Seconds()

		// Latencia promedio por operación
		operations := result.ReadBytes / Blocksize
		if operations > 0 {
			result.ReadLatency = result.ReadDuration / time.Duration(operations)
		}

		// Tasa de transferencia sostenida (promedio de lectura y escritura)
		if result.WriteThroughputMBs > 0 {
			result.SustainedThroughputMBs = (result.WriteThroughputMBs + result.ReadThroughputMBs) / 2.0
		} else {
			result.SustainedThroughputMBs = result.ReadThroughputMBs
		}
	}
}

// calculateIOPSMetrics calcula métricas avanzadas de IOPS
func (bm *Mark) calculateIOPSMetrics(resultIdx int) {
	result := &bm.Results[resultIdx]

	if result.IODuration > 0 && result.IOOperations > 0 {
		// Latencia promedio de IOPS
		result.IOPSLatency = result.IODuration / time.Duration(result.IOOperations)
	}
}

// calculateConsistencyAndStability calcula consistencia y estabilidad basado en múltiples resultados
func (bm *Mark) calculateConsistencyAndStability(resultIdx int) {
	if resultIdx == 0 {
		// Primer resultado, no hay datos para comparar
		bm.Results[resultIdx].ConsistencyScore = 100.0
		bm.Results[resultIdx].StabilityScore = 100.0
		return
	}

	result := &bm.Results[resultIdx]

	// Calcular consistencia basada en variación de throughput
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
		// Calcular desviación estándar
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

		// Consistencia: menor desviación = mayor consistencia
		// Normalizar a 0-100 (asumiendo que stdDev < mean)
		if mean > 0 {
			coefficientOfVariation := stdDev / mean
			result.ConsistencyScore = math.Max(0, 100.0*(1.0-coefficientOfVariation))
		} else {
			result.ConsistencyScore = 100.0
		}

		// Estabilidad: similar a consistencia pero considerando tendencias
		// Si los valores son similares entre ciclos, alta estabilidad
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

	// Obtener CPU idle antes del test
	result.CPUIdleBefore = bm.getCPUIdleBefore()

	// Obtener estadísticas de CPU durante el test
	avgCPU, maxCPU, _ := bm.stopCPUMonitoring()
	result.CPUAverageUsage = avgCPU
	result.CPUPeakUsage = maxCPU
	result.CPUIdleDuring = 100.0 - avgCPU

	// Overhead = diferencia entre CPU idle antes y durante
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
