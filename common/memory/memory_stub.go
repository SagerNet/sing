//go:build (darwin && !cgo) || (!darwin && !linux && !windows)

package memory

func totalNative() uint64 {
	return 0
}

func totalAvailable() bool {
	return false
}

func limitNative() uint64 {
	return 0
}

func limitAvailable() bool {
	return false
}

func availableNative() uint64 {
	return 0
}

func availableAvailable() bool {
	return false
}
