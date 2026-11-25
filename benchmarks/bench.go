package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Result struct {
	Label       string
	ArchiveSize int64
	Compress    time.Duration
	Extract     time.Duration
	PeakRSSMB   float64
}

type RunMetrics struct {
	Compress    time.Duration
	Extract     time.Duration
	ArchiveSize int64
	PeakRSSMB   float64
}

type Benchmark func(archivePath, input, output string) (RunMetrics, error)

func maxRSSMB(cmd *exec.Cmd) float64 {
	if cmd == nil || cmd.ProcessState == nil {
		return 0
	}
	if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok && ru != nil {
		// On Linux, Maxrss is reported in kilobytes.
		return float64(ru.Maxrss) / 1024.0
	}
	return 0
}

func runCommandDir(dir, name string, args ...string) (float64, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("%s %v: %w\n%s", name, args, err, out)
	}
	return maxRSSMB(cmd), nil
}

func requireTools(names ...string) map[string]bool {
	available := make(map[string]bool, len(names))
	for _, name := range names {
		_, err := exec.LookPath(name)
		available[name] = err == nil
		if err != nil {
			fmt.Printf("Skipping benchmarks requiring %q (not found in PATH)\n", name)
		}
	}
	return available
}

func fileSHA256(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func collectManifest(root string) (map[string][32]byte, error) {
	manifest := make(map[string][32]byte)
	base := filepath.Base(filepath.Clean(root))
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("benchmark dataset contains symlink: %s", path)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(base, rel))
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		manifest[key] = sum
		return nil
	})
	return manifest, err
}

func verifyExtracted(manifest map[string][32]byte, outputDir string) error {
	for rel, expected := range manifest {
		extractedPath := filepath.Join(outputDir, filepath.FromSlash(rel))
		actual, err := fileSHA256(extractedPath)
		if err != nil {
			return fmt.Errorf("verify %s: %w", rel, err)
		}
		if actual != expected {
			return fmt.Errorf("verify %s: SHA-256 mismatch", rel)
		}
	}
	return nil
}

func runPulsar(binary, archivePath, input, output string, version int, codec string, chunkSizeKB, workers int) (RunMetrics, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)

	archiveName := strings.TrimSuffix(
		filepath.Base(archivePath),
		filepath.Ext(archivePath),
	)
	actualArchivePath := filepath.Join(
		filepath.Dir(archivePath),
		archiveName+".plsr",
	)

	_ = os.Remove(actualArchivePath)
	defer os.Remove(actualArchivePath)

	createArgs := []string{
		"create", archiveName, input,
		"-o", filepath.Dir(archivePath),
		"--workers", fmt.Sprint(workers),
		"-v", fmt.Sprint(version),
		"--codec", codec,
	}
	if version == 2 {
		createArgs = append(createArgs, "--chunk-size", fmt.Sprint(chunkSizeKB))
	}

	start := time.Now()
	createCmd := exec.Command(binary, createArgs...)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return RunMetrics{}, fmt.Errorf("Pulsar v%d (%s) create: %w\n%s", version, codec, err, out)
	}
	compressTime := time.Since(start)
	createRSS := maxRSSMB(createCmd)

	info, err := os.Stat(actualArchivePath)
	if err != nil {
		return RunMetrics{}, fmt.Errorf("expected archive %s: %w", actualArchivePath, err)
	}

	start = time.Now()
	extractCmd := exec.Command(
		binary, "extract", actualArchivePath,
		"-o", output, "--workers", fmt.Sprint(workers),
	)
	if out, err := extractCmd.CombinedOutput(); err != nil {
		return RunMetrics{}, fmt.Errorf("Pulsar v%d (%s) extract: %w\n%s", version, codec, err, out)
	}
	extractTime := time.Since(start)
	extractRSS := maxRSSMB(extractCmd)

	peakRSS := createRSS
	if extractRSS > peakRSS {
		peakRSS = extractRSS
	}

	return RunMetrics{
		Compress:    compressTime,
		Extract:     extractTime,
		ArchiveSize: info.Size(),
		PeakRSSMB:   peakRSS,
	}, nil
}

func benchmarkExternal(
	archivePath, input, output, workDir string,
	createArgs, extractArgs func(archivePath, input, output string) []string,
) (RunMetrics, error) {
	_ = os.Remove(archivePath)
	_ = os.RemoveAll(output)

	start := time.Now()
	create := createArgs(archivePath, input, output)
	createRSS, err := runCommandDir(workDir, create[0], create[1:]...)
	if err != nil {
		return RunMetrics{}, err
	}
	compressTime := time.Since(start)

	info, err := os.Stat(archivePath)
	if err != nil {
		return RunMetrics{}, err
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return RunMetrics{}, err
	}
	start = time.Now()
	args := extractArgs(archivePath, input, output)
	extractRSS, err := runCommandDir(workDir, args[0], args[1:]...)
	if err != nil {
		return RunMetrics{}, err
	}
	extractTime := time.Since(start)

	peakRSS := createRSS
	if extractRSS > peakRSS {
		peakRSS = extractRSS
	}

	return RunMetrics{
		Compress:    compressTime,
		Extract:     extractTime,
		ArchiveSize: info.Size(),
		PeakRSSMB:   peakRSS,
	}, nil
}

func medianDuration(values []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func medianInt64(values []int64) int64 {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func maxFloat64(values []float64) float64 {
	var m float64
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return m
}

func totalSize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("benchmark dataset contains symlink: %s", path)
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func parseIntList(raw string, allowZero bool) ([]int, error) {
	parts := strings.Split(raw, ",")
	vals := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		val, err := strconv.Atoi(p)
		if err != nil || val < 0 || (!allowZero && val == 0) {
			return nil, fmt.Errorf("invalid integer value %q", p)
		}
		vals = append(vals, val)
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("at least one value is required")
	}
	return vals, nil
}

func parseStringList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printResult(r Result, originalSize int64) {
	ratio := float64(r.ArchiveSize) / float64(originalSize) * 100
	compressSpeed := float64(originalSize) / r.Compress.Seconds() / (1024 * 1024)
	extractSpeed := float64(originalSize) / r.Extract.Seconds() / (1024 * 1024)
	rssCol := "-"
	if r.PeakRSSMB > 0 {
		rssCol = fmt.Sprintf("%.1f", r.PeakRSSMB)
	}
	fmt.Printf("%-32s %-11.2f %-9.2f %-10.2f %-11.2f %-10.2f %-11.2f %-9s\n",
		r.Label,
		float64(r.ArchiveSize)/(1024*1024),
		ratio,
		r.Compress.Seconds(),
		compressSpeed,
		r.Extract.Seconds(),
		extractSpeed,
		rssCol,
	)
}

func runRepeated(label string, runs int, originalSize int64, manifest map[string][32]byte,
	tempDir, input string, benchmark Benchmark) error {
	var compressTimes, extractTimes []time.Duration
	var archiveSizes []int64
	var peakRSSValues []float64

	safeID := strings.NewReplacer(" ", "-", "/", "-", "(", "", ")", "", ",", "-").Replace(label)

	for run := 0; run < runs; run++ {
		archivePath := filepath.Join(tempDir, fmt.Sprintf("bench-%s-%d.archive", safeID, run))
		output := filepath.Join(tempDir, fmt.Sprintf("out-%s-%d", safeID, run))

		var runErr error
		func() {
			defer os.RemoveAll(output)
			defer os.Remove(archivePath)

			m, err := benchmark(archivePath, input, output)
			if err != nil {
				runErr = fmt.Errorf("%s run %d: %w", label, run+1, err)
				return
			}
			if manifest != nil {
				if err := verifyExtracted(manifest, output); err != nil {
					runErr = fmt.Errorf("%s run %d verification failed: %w", label, run+1, err)
					return
				}
			}
			compressTimes = append(compressTimes, m.Compress)
			extractTimes = append(extractTimes, m.Extract)
			archiveSizes = append(archiveSizes, m.ArchiveSize)
			peakRSSValues = append(peakRSSValues, m.PeakRSSMB)
		}()
		if runErr != nil {
			return runErr
		}
	}

	printResult(Result{
		Label:       label,
		ArchiveSize: medianInt64(archiveSizes),
		Compress:    medianDuration(compressTimes),
		Extract:     medianDuration(extractTimes),
		PeakRSSMB:   maxFloat64(peakRSSValues),
	}, originalSize)
	return nil
}

func benchmarkDataset(binaryPath, inputRaw string, runs int, chunkSizes, workerCounts []int, verify bool, available map[string]bool) error {
	inputPath, err := filepath.Abs(inputRaw)
	if err != nil {
		return err
	}
	info, err := os.Stat(inputPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("input %q must be an existing directory", inputRaw)
	}

	originalSize, err := totalSize(inputPath)
	if err != nil {
		return err
	}
	if originalSize == 0 {
		return fmt.Errorf("dataset %q contains no data", inputPath)
	}

	var manifest map[string][32]byte
	if verify {
		manifest, err = collectManifest(inputPath)
		if err != nil {
			return fmt.Errorf("failed to build verification manifest: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "pulsar-benchmark-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	fmt.Printf("\n========================================================================================================\n")
	fmt.Printf("Dataset: %s (%.2f MB) | Runs: %d | Workers: %v | V2 Chunks (KB): %v | Verify: %v\n",
		inputPath, float64(originalSize)/(1024*1024), runs, workerCounts, chunkSizes, verify)
	fmt.Printf("========================================================================================================\n")
	fmt.Printf("%-32s %-11s %-9s %-10s %-11s %-10s %-11s %-9s\n",
		"Configuration", "Archive MB", "Ratio %", "Comp (s)", "Comp MB/s", "Ext (s)", "Ext MB/s", "Peak RSS")
	fmt.Println(strings.Repeat("-", 104))

	pulsarBench := func(version int, codec string, cKB, workers int) Benchmark {
		return func(a, i, o string) (RunMetrics, error) {
			return runPulsar(binaryPath, a, i, o, version, codec, cKB, workers)
		}
	}

	codecs := []string{"deflate", "zstd"}

	// Pulsar V1 matrix
	for _, codec := range codecs {
		for _, workers := range workerCounts {
			label := fmt.Sprintf("PulsarV1-%s (%dw)", codec, workers)
			if err := runRepeated(label, runs, originalSize, manifest, tempDir, inputPath, pulsarBench(1, codec, 0, workers)); err != nil {
				return err
			}
		}
	}

	// Pulsar V2 matrix
	for _, cKB := range chunkSizes {
		chunkLabel := fmt.Sprintf("%dKB", cKB)
		if cKB == 0 {
			chunkLabel = "dyn"
		}
		for _, codec := range codecs {
			for _, workers := range workerCounts {
				label := fmt.Sprintf("PulsarV2-%s (%s, %dw)", codec, chunkLabel, workers)
				if err := runRepeated(label, runs, originalSize, manifest, tempDir, inputPath, pulsarBench(2, codec, cKB, workers)); err != nil {
					return err
				}
			}
		}
	}

	parent, base := filepath.Dir(inputPath), filepath.Base(inputPath)

	// Reference baselines
	if available["tar"] && available["zstd"] {
		bench := func(a, _, o string) (RunMetrics, error) {
			return benchmarkExternal(a, inputPath, o, parent,
				func(a, _, _ string) []string {
					return []string{"tar", "-I", "zstd -T0 -3", "-cf", a, "-C", parent, base}
				},
				func(a, _, o string) []string {
					return []string{"tar", "-I", "zstd -d", "-xf", a, "-C", o}
				})
		}
		if err := runRepeated("tar + zstd (-T0 -3)", runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
			return err
		}
	}

	if available["tar"] && available["pigz"] {
		bench := func(a, _, o string) (RunMetrics, error) {
			return benchmarkExternal(a, inputPath, o, parent,
				func(a, _, _ string) []string {
					return []string{"tar", "-I", "pigz -6", "-cf", a, "-C", parent, base}
				},
				func(a, _, o string) []string {
					return []string{"tar", "-I", "pigz -d", "-xf", a, "-C", o}
				})
		}
		if err := runRepeated("tar + pigz (-6)", runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
			return err
		}
	} else if available["tar"] && available["gzip"] {
		bench := func(a, _, o string) (RunMetrics, error) {
			return benchmarkExternal(a, inputPath, o, parent,
				func(a, _, _ string) []string {
					return []string{"tar", "-czf", a, "-C", parent, base}
				},
				func(a, _, o string) []string {
					return []string{"tar", "-xzf", a, "-C", o}
				})
		}
		if err := runRepeated("tar + gzip", runs, originalSize, manifest, tempDir, inputPath, bench); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	binary := flag.String("plsr", "./plsr", "Path to Pulsar executable")
	input := flag.String("input", "", "Comma-separated input directories to benchmark")
	runs := flag.Int("runs", 3, "Number of benchmark repetitions")
	chunkSizesFlag := flag.String("chunk-sizes", "1024,4096,8192,0", "Comma-separated chunk sizes in KB for Pulsar V2 (0 = dynamic)")
	workersFlag := flag.String("workers", "4,8,12", "Comma-separated worker counts to benchmark")
	verify := flag.Bool("verify", true, "Verify extracted files SHA-256 byte-for-byte against input")
	flag.Parse()

	if *input == "" || *runs <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	datasets := parseStringList(*input)
	if len(datasets) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	chunkSizes, err := parseIntList(*chunkSizesFlag, true)
	if err != nil {
		panic(err)
	}
	workerCounts, err := parseIntList(*workersFlag, false)
	if err != nil {
		panic(err)
	}

	binaryPath, err := filepath.Abs(*binary)
	if err != nil {
		panic(err)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		panic(fmt.Errorf("Pulsar binary not found: %w", err))
	}

	available := requireTools("tar", "zstd", "pigz", "gzip")

	for _, ds := range datasets {
		if err := benchmarkDataset(binaryPath, ds, *runs, chunkSizes, workerCounts, *verify, available); err != nil {
			panic(err)
		}
	}
}
