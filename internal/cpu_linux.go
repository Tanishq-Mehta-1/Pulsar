package archive

//linux based fetching for cpu cores
import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func PhysicalCoresLinux() (int, error) {
	cpus, err := filepath.Glob("sys/devices/system/cpu/cpu[0-9]*")
	if err != nil {
		return 0, err
	}

	uniqueCores := make(map[string]struct{})
	for _, cpuDir := range cpus {
		pkgBytes, err1 := os.ReadFile(filepath.Join(cpuDir, "topology/physical_package_id"))
		coreBytes, err2 := os.ReadFile(filepath.Join(cpuDir, "topology/core_id"))
		if err1 != nil || err2 != nil {
			continue //skip this cpu
		}

		id := strings.TrimSpace(string(pkgBytes)) + ":" + strings.TrimSpace(string(coreBytes))
		uniqueCores[id] = struct{}{}

	}

	return len(uniqueCores), nil
}

func LogicalCores() int {
	return max(1, runtime.GOMAXPROCS(0))
}

func ResolveWorkers(requested int) int {
	if requested > 0 {
		return requested
	}

	// cores, _ := PhysicalCoresLinux()
	// return cores

	return LogicalCores()
}
