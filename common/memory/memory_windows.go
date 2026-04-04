package memory

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func totalNative() uint64 {
	var mem processMemoryCounters
	mem.cb = uint32(unsafe.Sizeof(mem))
	err := getProcessMemoryInfo(windows.CurrentProcess(), &mem, mem.cb)
	if err != nil {
		return 0
	}
	return uint64(mem.workingSetSize)
}

func totalAvailable() bool {
	return true
}

func availableNative() uint64 {
	var mem memoryStatusEx
	mem.dwLength = uint32(unsafe.Sizeof(mem))
	err := globalMemoryStatusEx(&mem)
	if err != nil {
		return 0
	}
	return mem.ullAvailPhys
}

func availableAvailable() bool {
	return true
}
