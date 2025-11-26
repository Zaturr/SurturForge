//go:build !windows
// +build !windows

package ram

import (
	"fmt"
	"syscall"
)

type OptimizationResult struct {
	MemoryLocked    bool
	HighPriority    bool
	MemoryLockError string
	PriorityError   string
}

func lockMemory(buffer []byte) error {
	if len(buffer) == 0 {
		return nil
	}
	return syscall.Mlock(buffer)
}

func unlockMemory(buffer []byte) error {
	if len(buffer) == 0 {
		return nil
	}
	return syscall.Munlock(buffer)
}

func setHighPriority() error {
	err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, -20)
	if err != nil {
		err = syscall.Setpriority(syscall.PRIO_PROCESS, 0, -10)
		if err != nil {
			return fmt.Errorf("failed to set priority: %w (may require root/admin)", err)
		}
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
