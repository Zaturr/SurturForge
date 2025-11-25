package disk

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// DiskStressConfig configuración para el test de estrés del disco
type DiskStressConfig struct {
	DBPath             string
	WriteWorkers       int
	ReadWorkers        int
	DeleteWorkers      int // Workers para operaciones DELETE
	OperationsPerCycle int
	TestDuration       time.Duration
	DataSize           int
}

// DiskStressResult resultados del test de estrés
type DiskStressResult struct {
	TotalWrites       int64
	TotalReads        int64
	TotalCycles       int64
	TotalBytesWritten int64
	TotalBytesRead    int64
	WriteLatency      time.Duration
	ReadLatency       time.Duration
	AverageWriteTime  time.Duration
	AverageReadTime   time.Duration
	Errors            int64
	StartTime         time.Time
	EndTime           time.Time
}

// RunDiskStress ejecuta un test de estrés del disco usando SQLite sin WAL
func RunDiskStress(config DiskStressConfig) (*DiskStressResult, error) {
	result := &DiskStressResult{
		StartTime: time.Now(),
	}

	// Asegurar que el directorio existe
	if err := os.MkdirAll(filepath.Dir(config.DBPath), 0755); err != nil {
		return nil, fmt.Errorf("error al crear directorio: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)
	defer cancel()

	var writeCount int64
	var readCount int64
	var writeErrors int64
	var readErrors int64
	var cycleCount int64
	var totalWriteTime int64 // en nanosegundos
	var totalReadTime int64  // en nanosegundos
	var totalBytesWritten int64
	var totalBytesRead int64
	var totalOperations int64 // Contador total de operaciones (escrituras + lecturas)

	// Canal para coordinar el borrado del archivo
	resetChan := make(chan struct{}, 1)
	var resetMutex sync.Mutex

	// Función para resetear la base de datos
	resetDB := func() error {
		resetMutex.Lock()
		defer resetMutex.Unlock()

		// Esperar un momento para que las operaciones en curso terminen
		time.Sleep(100 * time.Millisecond)

		// Borrar el archivo - esto genera I/O en disco
		if err := os.Remove(config.DBPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("error al borrar archivo: %w", err)
		}

		// También borrar archivos temporales de SQLite si existen
		os.Remove(config.DBPath + "-wal")
		os.Remove(config.DBPath + "-shm")
		os.Remove(config.DBPath + "-journal")

		// Crear nueva base de datos
		db, err := sql.Open("sqlite", config.DBPath+"?_journal_mode=DELETE&_synchronous=FULL")
		if err != nil {
			return fmt.Errorf("error al abrir base de datos: %w", err)
		}
		defer db.Close()

		// Configurar para máximo estrés en disco
		db.Exec("PRAGMA synchronous = FULL")
		db.Exec("PRAGMA journal_mode = DELETE")
		db.Exec("PRAGMA cache_size = 0") // Deshabilitar caché para forzar I/O

		// Crear tabla
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS stress_test (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				data BLOB,
				timestamp INTEGER
			)
		`)
		if err != nil {
			return fmt.Errorf("error al crear tabla: %w", err)
		}

		atomic.AddInt64(&cycleCount, 1)
		return nil
	}

	// Inicializar la base de datos
	if err := resetDB(); err != nil {
		return nil, err
	}

	// Worker de escritura
	writeWorker := func(workerID int) {
		// Usar conexión persistente para mejor rendimiento y estrés
		db, err := sql.Open("sqlite", config.DBPath+"?_journal_mode=DELETE&_synchronous=FULL&_busy_timeout=5000")
		if err != nil {
			atomic.AddInt64(&writeErrors, 1)
			return
		}
		defer db.Close()

		// Configurar para forzar escritura inmediata al disco
		db.Exec("PRAGMA synchronous = FULL")
		db.Exec("PRAGMA journal_mode = DELETE")

		data := make([]byte, config.DataSize)
		for i := range data {
			data[i] = byte((workerID + i) % 256)
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				start := time.Now()

				// Insertar datos - esto fuerza escritura al disco
				_, err = db.Exec(
					"INSERT INTO stress_test (data, timestamp) VALUES (?, ?)",
					data,
					time.Now().UnixNano(),
				)

				elapsed := time.Since(start)

				if err != nil {
					atomic.AddInt64(&writeErrors, 1)
					// Reintentar con nueva conexión si hay error de bloqueo
					time.Sleep(10 * time.Millisecond)
					continue
				}

				atomic.AddInt64(&writeCount, 1)
				atomic.AddInt64(&totalWriteTime, int64(elapsed))
				atomic.AddInt64(&totalBytesWritten, int64(len(data)))

				// Verificar si necesitamos resetear basándose en el total de operaciones
				totalOps := atomic.AddInt64(&totalOperations, 1)
				if totalOps%int64(config.OperationsPerCycle) == 0 {
					select {
					case resetChan <- struct{}{}:
					default:
					}
				}
			}
		}
	}

	// Worker de lectura
	readWorker := func(workerID int) {
		// Usar conexión persistente
		db, err := sql.Open("sqlite", config.DBPath+"?_journal_mode=DELETE&_synchronous=FULL&_busy_timeout=5000")
		if err != nil {
			atomic.AddInt64(&readErrors, 1)
			return
		}
		defer db.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				start := time.Now()

				// Leer un registro aleatorio - esto genera I/O en disco
				var data []byte
				var timestamp int64
				err = db.QueryRow(
					"SELECT data, timestamp FROM stress_test ORDER BY RANDOM() LIMIT 1",
				).Scan(&data, &timestamp)

				elapsed := time.Since(start)

				if err != nil {
					// Si no hay datos, no es un error crítico, solo continuar
					if err == sql.ErrNoRows {
						time.Sleep(10 * time.Millisecond) // Pequeña pausa si no hay datos
						continue
					}
					atomic.AddInt64(&readErrors, 1)
					time.Sleep(10 * time.Millisecond)
					continue
				}

				atomic.AddInt64(&readCount, 1)
				atomic.AddInt64(&totalReadTime, int64(elapsed))
				atomic.AddInt64(&totalBytesRead, int64(len(data)))

				// Verificar si necesitamos resetear basándose en el total de operaciones
				totalOps := atomic.AddInt64(&totalOperations, 1)
				if totalOps%int64(config.OperationsPerCycle) == 0 {
					select {
					case resetChan <- struct{}{}:
					default:
					}
				}
			}
		}
	}

	// Worker de eliminación (DELETE) - agrega más estrés al disco
	deleteWorker := func(workerID int) {
		// Usar conexión persistente
		db, err := sql.Open("sqlite", config.DBPath+"?_journal_mode=DELETE&_synchronous=FULL&_busy_timeout=5000")
		if err != nil {
			return
		}
		defer db.Close()

		// Configurar para forzar escritura inmediata al disco
		db.Exec("PRAGMA synchronous = FULL")
		db.Exec("PRAGMA journal_mode = DELETE")

		for {
			select {
			case <-ctx.Done():
				return
			default:
				start := time.Now()

				// Eliminar un registro aleatorio - esto genera escritura al disco
				result, err := db.Exec(
					"DELETE FROM stress_test WHERE id IN (SELECT id FROM stress_test ORDER BY RANDOM() LIMIT 1)",
				)

				elapsed := time.Since(start)

				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				rowsAffected, _ := result.RowsAffected()
				if rowsAffected > 0 {
					atomic.AddInt64(&writeCount, 1) // Contar DELETE como escritura
					atomic.AddInt64(&totalWriteTime, int64(elapsed))

					// Verificar si necesitamos resetear
					totalOps := atomic.AddInt64(&totalOperations, 1)
					if totalOps%int64(config.OperationsPerCycle) == 0 {
						select {
						case resetChan <- struct{}{}:
						default:
						}
					}
				} else {
					// No hay registros para eliminar, esperar un poco
					time.Sleep(50 * time.Millisecond)
				}
			}
		}
	}

	// Goroutine para manejar reseteos
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-resetChan:
				if err := resetDB(); err != nil {
					atomic.AddInt64(&writeErrors, 1)
				}
			}
		}
	}()

	// Iniciar workers de escritura
	var wg sync.WaitGroup
	for i := 0; i < config.WriteWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			writeWorker(id)
		}(i)
	}

	// Iniciar workers de lectura
	for i := 0; i < config.ReadWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			readWorker(id)
		}(i)
	}

	// Iniciar workers de eliminación (si está configurado)
	deleteWorkers := config.DeleteWorkers
	if deleteWorkers <= 0 {
		// Por defecto, usar la mitad de los workers de escritura
		deleteWorkers = config.WriteWorkers / 2
		if deleteWorkers == 0 {
			deleteWorkers = 1
		}
	}

	for i := 0; i < deleteWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deleteWorker(id)
		}(i)
	}

	// Esperar a que termine el contexto o se complete el tiempo
	<-ctx.Done()

	// Esperar a que todos los workers terminen
	wg.Wait()

	result.EndTime = time.Now()
	result.TotalWrites = atomic.LoadInt64(&writeCount)
	result.TotalReads = atomic.LoadInt64(&readCount)
	result.TotalCycles = atomic.LoadInt64(&cycleCount)
	result.TotalBytesWritten = atomic.LoadInt64(&totalBytesWritten)
	result.TotalBytesRead = atomic.LoadInt64(&totalBytesRead)
	result.Errors = atomic.LoadInt64(&writeErrors) + atomic.LoadInt64(&readErrors)

	if result.TotalWrites > 0 {
		avgWriteTime := atomic.LoadInt64(&totalWriteTime) / result.TotalWrites
		result.AverageWriteTime = time.Duration(avgWriteTime)
	}

	if result.TotalReads > 0 {
		avgReadTime := atomic.LoadInt64(&totalReadTime) / result.TotalReads
		result.AverageReadTime = time.Duration(avgReadTime)
	}

	// Limpiar archivo al finalizar
	os.Remove(config.DBPath)

	return result, nil
}

// PrintDiskStressResult imprime los resultados del test de estrés
func PrintDiskStressResult(result *DiskStressResult) {
	fmt.Println("\n=== Resultados del Test de Estrés del Disco ===")
	fmt.Printf("Duración: %v\n", result.EndTime.Sub(result.StartTime))
	fmt.Printf("Total de Escrituras: %d\n", result.TotalWrites)
	fmt.Printf("Total de Lecturas: %d\n", result.TotalReads)
	fmt.Printf("Total de Ciclos (Reseteos): %d\n", result.TotalCycles)
	fmt.Printf("Total de Bytes Escritos: %s\n", formatBytes(uint64(result.TotalBytesWritten)))
	fmt.Printf("Total de Bytes Leídos: %s\n", formatBytes(uint64(result.TotalBytesRead)))
	fmt.Printf("Tiempo Promedio de Escritura: %v\n", result.AverageWriteTime)
	fmt.Printf("Tiempo Promedio de Lectura: %v\n", result.AverageReadTime)

	if result.TotalWrites > 0 {
		writeThroughput := float64(result.TotalBytesWritten) / result.EndTime.Sub(result.StartTime).Seconds() / (1024 * 1024)
		fmt.Printf("Throughput de Escritura: %.2f MB/s\n", writeThroughput)
	}

	if result.TotalReads > 0 {
		readThroughput := float64(result.TotalBytesRead) / result.EndTime.Sub(result.StartTime).Seconds() / (1024 * 1024)
		fmt.Printf("Throughput de Lectura: %.2f MB/s\n", readThroughput)
	}

	if result.TotalWrites > 0 && result.TotalReads > 0 {
		totalThroughput := float64(result.TotalBytesWritten+result.TotalBytesRead) / result.EndTime.Sub(result.StartTime).Seconds() / (1024 * 1024)
		fmt.Printf("Throughput Total: %.2f MB/s\n", totalThroughput)
	}

	fmt.Printf("Errores: %d\n", result.Errors)
	fmt.Println("===============================================")
}
