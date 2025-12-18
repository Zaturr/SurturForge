package cpu

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	rtDataSizeMB    = 16
	rtDataSizeBytes = rtDataSizeMB * 1024 * 1024
	rtBlockSizeMB   = 1
	rtBlockSize     = rtBlockSizeMB * 1024 * 1024
	rtNumBlocks     = 16
)

// RunGoStyleBenchmarks ejecuta benchmarks de CPU dentro del binario (sin `go test`)
// y devuelve un output compatible con el parser existente (ParseBenchmarkOutput).
//
// pattern soporta los usados en main:
// - "^Benchmark.*" -> ejecuta todos
// - "^BenchmarkXYZ$" -> ejecuta solo ese benchmark
func RunGoStyleBenchmarks(pattern string, targetDuration time.Duration) (string, error) {
	names := resolveBenchmarkPattern(pattern)
	if len(names) == 0 {
		return "", fmt.Errorf("pattern de benchmark no soportado: %q", pattern)
	}

	var b strings.Builder
	for _, name := range names {
		iterations, nsPerOp, err := runOneBenchmark(name, targetDuration)
		if err != nil {
			return "", err
		}
		// Formato mínimo que ParseBenchmarkOutput entiende:
		// BenchmarkName-N  <iters>  <ns/op>
		fmt.Fprintf(&b, "%s-%d\t%d\t%d ns/op\n", name, runtime.GOMAXPROCS(0), iterations, nsPerOp)
	}
	return b.String(), nil
}

func resolveBenchmarkPattern(pattern string) []string {
	all := []string{
		"BenchmarkSHA256SingleCore",
		"BenchmarkAES256SingleCore",
		"BenchmarkSHA256MultiCore",
		"BenchmarkAES256MultiCore",
	}

	if pattern == "^Benchmark.*" {
		return all
	}

	// Esperamos formato exacto: ^Name$
	if strings.HasPrefix(pattern, "^") && strings.HasSuffix(pattern, "$") {
		name := strings.TrimSuffix(strings.TrimPrefix(pattern, "^"), "$")
		for _, n := range all {
			if n == name {
				return []string{name}
			}
		}
	}
	return nil
}

func runOneBenchmark(name string, targetDuration time.Duration) (iterations int64, nsPerOp int64, err error) {
	if targetDuration <= 0 {
		targetDuration = 3 * time.Second
	}

	switch name {
	case "BenchmarkSHA256SingleCore":
		it, ns := benchSHA256SingleCore(targetDuration)
		return it, ns, nil
	case "BenchmarkAES256SingleCore":
		it, ns, e := benchAESSingleCore(targetDuration)
		return it, ns, e
	case "BenchmarkSHA256MultiCore":
		it, ns := benchSHA256MultiCore(targetDuration)
		return it, ns, nil
	case "BenchmarkAES256MultiCore":
		it, ns, e := benchAESMultiCore(targetDuration)
		return it, ns, e
	default:
		return 0, 0, fmt.Errorf("benchmark no soportado: %s", name)
	}
}

// Helpers:
// - Para SHA devolvemos ns/op calculado como (elapsed / iters)
// - Para AES igual, porque incluye overhead de GCM.Seal.

func benchSHA256SingleCore(d time.Duration) (iterations int64, nsPerOp int64) {
	data := make([]byte, rtDataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}
	hasher := sha256.New()

	start := time.Now()
	deadline := start.Add(d)
	var it int64
	for time.Now().Before(deadline) {
		hasher.Reset()
		_, _ = hasher.Write(data)
		_ = hasher.Sum(nil)
		it++
	}
	elapsed := time.Since(start)
	if it == 0 {
		return 1, elapsed.Nanoseconds()
	}
	return it, elapsed.Nanoseconds() / it
}

func benchAESSingleCore(d time.Duration) (iterations int64, nsPerOp int64, err error) {
	data := make([]byte, rtDataSizeBytes)
	for i := range data {
		data[i] = byte(i % 256)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return 0, 0, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, 0, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return 0, 0, err
	}
	ciphertext := make([]byte, 0, len(data)+gcm.Overhead())

	start := time.Now()
	deadline := start.Add(d)
	var it int64
	for time.Now().Before(deadline) {
		ciphertext = gcm.Seal(ciphertext[:0], nonce, data, nil)
		_ = ciphertext
		it++
	}
	elapsed := time.Since(start)
	if it == 0 {
		return 1, elapsed.Nanoseconds(), nil
	}
	return it, elapsed.Nanoseconds() / it, nil
}

func benchSHA256MultiCore(d time.Duration) (iterations int64, nsPerOp int64) {
	blocks := make([][]byte, rtNumBlocks)
	for i := 0; i < rtNumBlocks; i++ {
		blocks[i] = make([]byte, rtBlockSize)
		for j := range blocks[i] {
			blocks[i][j] = byte((i*rtBlockSize + j) % 256)
		}
	}

	var it atomic.Int64
	start := time.Now()
	deadline := start.Add(d)

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			hasher := sha256.New()
			for time.Now().Before(deadline) {
				for _, block := range blocks {
					hasher.Reset()
					_, _ = hasher.Write(block)
					_ = hasher.Sum(nil)
				}
				it.Add(1)
			}
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	total := it.Load()
	if total == 0 {
		return 1, elapsed.Nanoseconds()
	}
	return total, elapsed.Nanoseconds() / total
}

func benchAESMultiCore(d time.Duration) (iterations int64, nsPerOp int64, err error) {
	blocks := make([][]byte, rtNumBlocks)
	for i := 0; i < rtNumBlocks; i++ {
		blocks[i] = make([]byte, rtBlockSize)
		for j := range blocks[i] {
			blocks[i][j] = byte((i*rtBlockSize + j) % 256)
		}
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return 0, 0, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, 0, err
	}

	nonces := make([][]byte, rtNumBlocks)
	for i := 0; i < rtNumBlocks; i++ {
		nonces[i] = make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonces[i]); err != nil {
			return 0, 0, err
		}
	}

	var it atomic.Int64
	start := time.Now()
	deadline := start.Add(d)

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			ciphertexts := make([][]byte, rtNumBlocks)
			for i := 0; i < rtNumBlocks; i++ {
				ciphertexts[i] = make([]byte, 0, rtBlockSize+gcm.Overhead())
			}
			for time.Now().Before(deadline) {
				for i, block := range blocks {
					ciphertexts[i] = gcm.Seal(ciphertexts[i][:0], nonces[i], block, nil)
				}
				it.Add(1)
			}
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	total := it.Load()
	if total == 0 {
		return 1, elapsed.Nanoseconds(), nil
	}
	return total, elapsed.Nanoseconds() / total, nil
}
