package cpu

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

const (
	dataSizeMB = 16
	dataSizeBytes = dataSizeMB * 1024 * 1024
	blockSizeMB = 1
	blockSizeBytes = blockSizeMB * 1024 * 1024
	numBlocks = 16
)

func BenchmarkSHA256SingleCore(b *testing.B) {
	data := make([]byte, dataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}

	hasher := sha256.New()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hasher.Reset()
		hasher.Write(data)
		_ = hasher.Sum(nil)
	}
}

func BenchmarkAES256SingleCore(b *testing.B) {
	data := make([]byte, dataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}

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
		ciphertext = gcm.Seal(ciphertext[:0], nonce, data, nil)
		_ = ciphertext
	}
}

func BenchmarkSHA256MultiCore(b *testing.B) {
	blocks := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blocks[i] = make([]byte, blockSizeBytes)
		for j := range blocks[i] {
			blocks[i][j] = byte((i*blockSizeBytes + j) % 256)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		hasher := sha256.New()
		for pb.Next() {
			for _, block := range blocks {
				hasher.Reset()
				hasher.Write(block)
				_ = hasher.Sum(nil)
			}
		}
	})
}

func BenchmarkAES256MultiCore(b *testing.B) {
	blocks := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blocks[i] = make([]byte, blockSizeBytes)
		for j := range blocks[i] {
			blocks[i][j] = byte((i*blockSizeBytes + j) % 256)
		}
	}

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

	nonces := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		nonces[i] = make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonces[i]); err != nil {
			b.Fatalf("Error al generar el nonce %d: %v", i, err)
		}
	}

	ciphertexts := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		ciphertexts[i] = make([]byte, blockSizeBytes+gcm.Overhead())
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for i, block := range blocks {
				ciphertexts[i] = gcm.Seal(ciphertexts[i][:0], nonces[i], block, nil)
				_ = ciphertexts[i]
			}
		}
	})
}

func BenchmarkSHA256AndAES256Sequential(b *testing.B) {
	data := make([]byte, dataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}

	hasher := sha256.New()

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
		hasher.Reset()
		hasher.Write(data)
		_ = hasher.Sum(nil)

		ciphertext = gcm.Seal(ciphertext[:0], nonce, data, nil)
		_ = ciphertext
	}
}
