//go:build !windows
// +build !windows

package ram

func getRAMFrequencyWindows() uint64 {
	return 0
}

func getMemoryChannelsWindows() int {
	return 0
}
