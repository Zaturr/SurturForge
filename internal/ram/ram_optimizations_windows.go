//go:build windows
// +build windows

package ram

import (
	"fmt"
	"syscall"
	"unsafe"
)

type OptimizationResult struct {
	MemoryLocked    bool
	HighPriority    bool
	MemoryLockError string
	PriorityError   string
}

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procSetPriorityClass  = kernel32.NewProc("SetPriorityClass")
	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
	procVirtualLock       = kernel32.NewProc("VirtualLock")
	procVirtualUnlock     = kernel32.NewProc("VirtualUnlock")
)

const (
	HIGH_PRIORITY_CLASS = 0x00000080
)

func lockMemory(buffer []byte) error {
	if len(buffer) == 0 {
		return nil
	}
	ptr := unsafe.Pointer(&buffer[0])
	length := uintptr(len(buffer))
	ret, _, _ := procVirtualLock.Call(uintptr(ptr), length)
	if ret == 0 {
		return fmt.Errorf("VirtualLock failed")
	}
	return nil
}

func unlockMemory(buffer []byte) error {
	if len(buffer) == 0 {
		return nil
	}
	ptr := unsafe.Pointer(&buffer[0])
	length := uintptr(len(buffer))
	ret, _, _ := procVirtualUnlock.Call(uintptr(ptr), length)
	if ret == 0 {
		return fmt.Errorf("VirtualUnlock failed")
	}
	return nil
}

func setHighPriority() error {
	handle, _, _ := procGetCurrentProcess.Call()
	ret, _, _ := procSetPriorityClass.Call(handle, HIGH_PRIORITY_CLASS)
	if ret == 0 {
		return fmt.Errorf("SetPriorityClass failed (may require admin)")
	}
	return nil
}

func ApplyOptimizations(config RAMStressConfig, buffers [][]byte) *OptimizationResult {
	result := &OptimizationResult{}

	if config.EnableHighPriority {
		err := setHighPriority()
		if err != nil {
			result.PriorityError = err.Error()
		} else {
			result.HighPriority = true
		}
	}

	if config.EnableMemoryLock {
		allLocked := true
		for i, buffer := range buffers {
			err := lockMemory(buffer)
			if err != nil {
				allLocked = false
				if result.MemoryLockError == "" {
					result.MemoryLockError = fmt.Sprintf("failed to lock buffer %d: %v", i, err)
				}
			}
		}
		if allLocked {
			result.MemoryLocked = true
		}
	}

	return result
}

func CleanupOptimizations(config RAMStressConfig, buffers [][]byte) {
	if config.EnableMemoryLock {
		for _, buffer := range buffers {
			_ = unlockMemory(buffer)
		}
	}
}
