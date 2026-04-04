package memory

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
)

var pageSize = uint64(os.Getpagesize())

func totalNative() uint64 {
	fd, err := os.Open("/proc/self/statm")
	if err != nil {
		return 0
	}
	defer fd.Close()
	var buf [128]byte
	n, _ := fd.Read(buf[:])
	if n == 0 {
		return 0
	}
	i := 0
	for i < n && buf[i] != ' ' {
		i++
	}
	i++
	var rss uint64
	for i < n && buf[i] >= '0' && buf[i] <= '9' {
		rss = rss*10 + uint64(buf[i]-'0')
		i++
	}
	return rss * pageSize
}

func totalAvailable() bool {
	fd, err := os.Open("/proc/self/statm")
	if err != nil {
		return false
	}
	defer fd.Close()
	var buf [1]byte
	n, _ := fd.Read(buf[:])
	return n > 0
}

func availableNative() uint64 {
	available, ok := cgroupAvailable()
	if ok {
		return available
	}
	return procMemAvailable()
}

func availableAvailable() bool {
	_, ok := cgroupAvailable()
	if ok {
		return true
	}
	fd, err := os.Open("/proc/meminfo")
	if err != nil {
		return false
	}
	fd.Close()
	return true
}

func cgroupAvailable() (uint64, bool) {
	max, err := readCgroupUint("/sys/fs/cgroup/memory.max")
	if err == nil && max != math.MaxUint64 {
		current, err := readCgroupUint("/sys/fs/cgroup/memory.current")
		if err == nil && max > current {
			return max - current, true
		}
		return 0, true
	}
	limit, err := readCgroupUint("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err == nil && limit != math.MaxUint64 {
		usage, err := readCgroupUint("/sys/fs/cgroup/memory/memory.usage_in_bytes")
		if err == nil && limit > usage {
			return limit - usage, true
		}
		return 0, true
	}
	return 0, false
}

func readCgroupUint(path string) (uint64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(content))
	if text == "max" {
		return math.MaxUint64, nil
	}
	return strconv.ParseUint(text, 10, 64)
}

func procMemAvailable() uint64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err == nil {
					return value * 1024
				}
			}
			break
		}
	}
	return 0
}
