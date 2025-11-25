package cpu

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

const (
	// dataSizeMB es el tamaño total de datos en MB para las pruebas
	dataSizeMB = 16
	// dataSizeBytes es el tamaño total en bytes (16 MB = 16 * 1024 * 1024)
	dataSizeBytes = dataSizeMB * 1024 * 1024
	// blockSizeMB es el tamaño de cada bloque para procesamiento paralelo
	blockSizeMB = 1
	// blockSizeBytes es el tamaño de cada bloque en bytes (1 MB)
	blockSizeBytes = blockSizeMB * 1024 * 1024
	// numBlocks es el número de bloques para procesamiento paralelo (16 bloques de 1 MB)
	numBlocks = 16
)

// BenchmarkSHA256SingleCore ejecuta una prueba de hashing SHA-256 en un solo núcleo.
// Pre-asigna un buffer de 16 MB fuera del bucle b.N para evitar medir la asignación
// de memoria o el garbage collector de Go.
func BenchmarkSHA256SingleCore(b *testing.B) {
	// Pre-asignar el buffer de datos fuera del bucle b.N
	// Esto asegura que no medimos la asignación de memoria, solo el procesamiento
	data := make([]byte, dataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256) // Llenar con datos de prueba
	}

	// Pre-inicializar el hasher fuera del bucle para evitar overhead
	hasher := sha256.New()

	b.ResetTimer() // Resetear el timer después de la preparación
	b.ReportAllocs() // Reportar asignaciones de memoria

	for i := 0; i < b.N; i++ {
		hasher.Reset()
		hasher.Write(data)
		_ = hasher.Sum(nil) // Forzar el cálculo del hash
	}
}

// BenchmarkAES256SingleCore ejecuta una prueba de cifrado AES-256-GCM en un solo núcleo.
// Pre-asigna buffers y configura el cifrador fuera del bucle b.N.
func BenchmarkAES256SingleCore(b *testing.B) {
	// Pre-asignar el buffer de datos fuera del bucle b.N
	data := make([]byte, dataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Generar una clave AES-256 (32 bytes)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatalf("Error al generar la clave: %v", err)
	}

	// Crear el cifrador AES
	block, err := aes.NewCipher(key)
	if err != nil {
		b.Fatalf("Error al crear el cifrador AES: %v", err)
	}

	// Crear el cifrador GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		b.Fatalf("Error al crear el cifrador GCM: %v", err)
	}

	// Pre-asignar el nonce (12 bytes para GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		b.Fatalf("Error al generar el nonce: %v", err)
	}

	// Pre-asignar el buffer para el texto cifrado
	ciphertext := make([]byte, len(data)+gcm.Overhead())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Cifrar los datos
		ciphertext = gcm.Seal(ciphertext[:0], nonce, data, nil)
		_ = ciphertext // Usar el resultado para evitar optimizaciones del compilador
	}
}

// BenchmarkSHA256MultiCore ejecuta una prueba de hashing SHA-256 usando múltiples núcleos.
// Procesa 16 bloques de 1 MB simultáneamente usando b.RunParallel.
func BenchmarkSHA256MultiCore(b *testing.B) {
	// Pre-asignar los bloques de datos fuera del bucle b.N
	blocks := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blocks[i] = make([]byte, blockSizeBytes)
		for j := range blocks[i] {
			blocks[i][j] = byte((i*blockSizeBytes + j) % 256)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Usar RunParallel para ejecutar en múltiples núcleos
	b.RunParallel(func(pb *testing.PB) {
		// Cada goroutine procesa todos los bloques
		hasher := sha256.New()
		for pb.Next() {
			// Procesar los 16 bloques de 1 MB cada uno
			for _, block := range blocks {
				hasher.Reset()
				hasher.Write(block)
				_ = hasher.Sum(nil)
			}
		}
	})
}

// BenchmarkAES256MultiCore ejecuta una prueba de cifrado AES-256-GCM usando múltiples núcleos.
// Procesa 16 bloques de 1 MB simultáneamente usando b.RunParallel.
func BenchmarkAES256MultiCore(b *testing.B) {
	// Pre-asignar los bloques de datos fuera del bucle b.N
	blocks := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blocks[i] = make([]byte, blockSizeBytes)
		for j := range blocks[i] {
			blocks[i][j] = byte((i*blockSizeBytes + j) % 256)
		}
	}

	// Generar una clave AES-256 (32 bytes)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatalf("Error al generar la clave: %v", err)
	}

	// Crear el cifrador AES
	block, err := aes.NewCipher(key)
	if err != nil {
		b.Fatalf("Error al crear el cifrador AES: %v", err)
	}

	// Crear el cifrador GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		b.Fatalf("Error al crear el cifrador GCM: %v", err)
	}

	// Pre-asignar nonces para cada bloque
	nonces := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		nonces[i] = make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonces[i]); err != nil {
			b.Fatalf("Error al generar el nonce %d: %v", i, err)
		}
	}

	// Pre-asignar buffers para texto cifrado
	ciphertexts := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		ciphertexts[i] = make([]byte, blockSizeBytes+gcm.Overhead())
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Usar RunParallel para ejecutar en múltiples núcleos
	b.RunParallel(func(pb *testing.PB) {
		// Cada goroutine procesa todos los bloques
		for pb.Next() {
			// Procesar los 16 bloques de 1 MB cada uno simultáneamente
			for i, block := range blocks {
				ciphertexts[i] = gcm.Seal(ciphertexts[i][:0], nonces[i], block, nil)
				_ = ciphertexts[i] // Usar el resultado
			}
		}
	})
}

// BenchmarkSHA256AndAES256Sequential ejecuta una prueba combinada que ejecuta
// SHA-256 y AES-256-GCM secuencialmente en un solo núcleo, como se especifica
// en los requisitos para la prueba de un solo núcleo.
func BenchmarkSHA256AndAES256Sequential(b *testing.B) {
	// Pre-asignar el buffer de datos fuera del bucle b.N
	data := make([]byte, dataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Pre-configurar SHA-256
	hasher := sha256.New()

	// Pre-configurar AES-256-GCM
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatalf("Error al generar la clave: %v", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		b.Fatalf("Error al crear el cifrador AES: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		b.Fatalf("Error al crear el cifrador GCM: %v", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		b.Fatalf("Error al generar el nonce: %v", err)
	}

	ciphertext := make([]byte, len(data)+gcm.Overhead())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Ejecutar hashing SHA-256
		hasher.Reset()
		hasher.Write(data)
		_ = hasher.Sum(nil)

		// Ejecutar cifrado AES-256-GCM secuencialmente
		ciphertext = gcm.Seal(ciphertext[:0], nonce, data, nil)
		_ = ciphertext
	}
}

