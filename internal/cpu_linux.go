package archive

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func PhysicalCoresLinux() (int, error) {
	cpus, err := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*")
	if err != nil {
		return 0, err
	}

	uniqueCores := make(map[string]struct{})
	for _, cpuDir := range cpus {
		pkgBytes, err1 := os.ReadFile(filepath.Join(cpuDir, "topology/physical_package_id"))
		coreBytes, err2 := os.ReadFile(filepath.Join(cpuDir, "topology/core_id"))
		if err1 != nil || err2 != nil {
			continue // skip this cpu
		}

		id := strings.TrimSpace(string(pkgBytes)) + ":" + strings.TrimSpace(string(coreBytes))
		uniqueCores[id] = struct{}{}
	}

	return len(uniqueCores), nil
}

func LogicalCores() int {
	return max(1, runtime.GOMAXPROCS(0))
}

func DetectPhysicalCores() int {
	logical := LogicalCores()
	phys, err := PhysicalCoresLinux()
	if err != nil || phys <= 0 {
		phys = max(1, logical/2)
	}
	// Respect container/cgroup GOMAXPROCS limits
	return min(phys, logical)
}

func ResolveWorkers(requested int, codec string) int {
	if requested > 0 {
		return requested
	}

	logical := LogicalCores()
	phys := DetectPhysicalCores()

	switch codec {
	case "zstd":
		// 1 worker per physical core avoids L2/L3 cache thrashing on Zstd hash tables
		return min(phys, 8)
	case "deflate":
		// Deflate's 32 KB window scales across logical threads up to the single-writer I/O ceiling
		return min(logical, 12)
	case "stored":
		// Purely disk/memory bandwidth bound
		return min(phys, 4)
	default:
		return logical
	}
}
