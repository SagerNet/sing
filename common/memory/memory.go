package memory

import "runtime"

func Total() uint64 {
	return totalNative()
}

func TotalAvailable() bool {
	return totalAvailable()
}

func Available() uint64 {
	return availableNative()
}

func AvailableAvailable() bool {
	return availableAvailable()
}

func Limit() uint64 {
	return limitNative()
}

func LimitAvailable() bool {
	return limitAvailable()
}

func Inuse() uint64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.StackInuse + memStats.HeapInuse + memStats.HeapIdle - memStats.HeapReleased
}
