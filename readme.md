# Pulsar (`plsr`)

<p align="center">
  <strong>A fast, concurrent lossless file archiver written in Go.</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version" />
  <img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="MIT License" />
  <img src="https://img.shields.io/badge/Format-V1%20%7C%20V2%20Chunked-blueviolet?style=flat-square" alt="Archive Format" />
</p>

---

**Pulsar** (`plsr`) is a command-line archiving utility built around a custom binary archive format. It supports both **file-level (V1)** and **chunk-based (V2)** parallel compression and extraction, adaptive per-file dynamic chunking, power-of-2 buffer pooling, nested and empty directories, and CRC32 integrity verification.

## Features

- **Dual-Version Binary Format**
  - **V1 (File-Level):** Parallel per-file compression and extraction with a central directory.
  - **V2 (Chunk-Based):** Parallel chunk-level compression and extraction with a file table and chunk index, enabling multi-core scaling even on single large files and indexed random access.
- **Adaptive & Fixed Chunking:** Supports fixed chunk sizes (default `4096 KB` / `4 MB`) or adaptive per-file dynamic chunking (`--chunk-size 0`) that scales between `64 KB` and `4096 KB` based on file size and worker count.
- **Low-Overhead Memory Pooling:** Power-of-2 `sync.Pool` buffer buckets (`64 KB`–`4 MB`) minimize GC pressure and peak heap allocations during chunked I/O.
- **Automatic CPU Scaling:** Defaults `--workers 0` to the number of logical CPUs available on the system.
- **DEFLATE Compression:** Configurable compression levels with automatic fallback/storage handling.
- **Full Directory Support:** Preserves nested directory hierarchies and empty directories.
- **Integrity Verification:** Per-file (V1) and per-chunk (V2) CRC32 checksums verified during extraction.
- **Archive Inspection:** Built-in `list` and `info` commands to inspect archive contents and metadata without extracting payloads.
- **Benchmarking Harness:** Included benchmark suite comparing Pulsar against `zip`, `7z`, `tar + pigz`, and `tar + zstd`.

---

## Installation

### Build from Source

```bash
git clone https://github.com/Tanishq-Mehta-1/Pulsar.git
cd Pulsar
go build -o ./plsr ./cmd
```

### System-Wide Install (Optional)

```bash
sudo install plsr /usr/local/bin/plsr
```

---

## Usage

### Commands

| Command | Description |
| :--- | :--- |
| `plsr create <archive> <inputs...> [flags]` | Create a new `.plsr` archive from files and/or directories |
| `plsr extract <archive> [flags]` | Extract an archive (auto-detects V1/V2) |
| `plsr list <archive>` | List all files and directories in an archive |
| `plsr info <archive>` | Display archive metadata (version, entries, sizes, ratios) |
| `plsr help [command]` | Show general or command-specific help |

### Flags

| Flag | Commands | Default | Description |
| :--- | :--- | :--- | :--- |
| `-o <dir>` | `create`, `extract` | Current dir | Output directory for the archive or extracted files |
| `-v <1\|2>` | `create` | `1` | Archive format version (`1` = file-level, `2` = chunk-based) |
| `--workers <n>` | `create`, `extract` | `0` (Logical CPUs) | Number of concurrent worker goroutines |
| `--chunk-size <KB>` | `create` (V2) | `4096` (`4 MB`) | Chunk size in KB (`0` enables adaptive per-file chunking) |

### Examples

```bash
# Create a V1 archive using all logical CPUs
plsr create backup ./project

# Create a V2 chunk-based archive with adaptive per-file chunking
plsr create backup ./project -v 2 --chunk-size 0 --workers 8

# Create a V2 archive with a fixed 256 KB chunk size
plsr create backup ./project -v 2 --chunk-size 256 --workers 8

# Archive multiple inputs into a specific output directory
plsr create backup ./project ./notes.txt -o ./archives --workers 8

# Extract an archive (auto-detects V1/V2)
plsr extract backup.plsr -o ./output --workers 8

# List archive contents
plsr list backup.plsr

# Inspect archive metadata
plsr info backup.plsr
```

---

## Benchmarks

All benchmarks were averaged over **3 runs** with full SHA-256 extraction verification enabled.

### 1. Large Dataset (`testdata5GB` — 5.33 GB / 5,330.19 MB)

Sustained disk-bound workload comparing Pulsar's optimal codec defaults against `tar + zstd (-T0 -3)` and `tar + pigz (-6)`:

| Tool / Configuration | Workers | Chunk Size | Archive Size | Ratio | Compress | Comp Speed | Extract | Extract Speed |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **`PulsarV2-zstd` (Fast)** | 6 | 2048 KB | 3,345.49 MB | 62.77% | **10.01s** | **532.41 MB/s** | **9.40s** | **566.98 MB/s** |
| **`PulsarV2-zstd` (Balanced)** | 4 | 4096 KB | 3,296.44 MB | 61.84% | 10.63s | 501.40 MB/s | **9.03s** | **590.23 MB/s** |
| **`PulsarV2-zstd` (Max Ratio)** | 4 | 8192 KB | 3,253.97 MB | 61.05% | 10.74s | 496.17 MB/s | 10.02s | 531.99 MB/s |
| **`PulsarV2-deflate`** | 12 | 8192 KB | 3,352.94 MB | 62.90% | 12.99s | 410.32 MB/s | 13.03s | 409.11 MB/s |
| `tar + zstd (-T0 -3)` | Auto | Stream | 3,186.66 MB | 59.79% | 10.52s | 506.87 MB/s | 10.23s | 521.16 MB/s |
| `tar + pigz (-6)` | Auto | Stream | 3,246.94 MB | 60.92% | 28.72s | 185.57 MB/s | 20.83s | 255.84 MB/s |

### 2. Medium Dataset (`testdata655MB` — 655.35 MB)

Cache-resident workload comparing Pulsar V1 (file-level) and V2 (chunk-level) across optimal worker tiers:

| Tool / Configuration | Workers | Chunk Size | Archive Size | Ratio | Compress | Comp Speed | Extract | Extract Speed |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **`PulsarV2-zstd` (Max Extract)** | 4 | 1024 KB | 494.39 MB | 75.44% | 1.28s | 512.44 MB/s | **0.30s** | **2,208.32 MB/s** |
| **`PulsarV2-zstd` (Fast Comp)** | 6 | 1024 KB | 494.39 MB | 75.44% | **1.10s** | **598.46 MB/s** | 0.31s | 2,090.76 MB/s |
| **`PulsarV2-zstd` (Balanced)** | 6 | 8192 KB | 484.64 MB | 73.95% | 1.12s | 582.78 MB/s | 0.56s | 1,177.16 MB/s |
| **`PulsarV1-zstd` (File-Level)** | 4 | Per-File | 480.95 MB | 73.39% | 1.68s | 389.15 MB/s | 0.45s | 1,470.44 MB/s |
| **`PulsarV2-deflate`** | 12 | 4096 KB | 478.82 MB | 73.06% | 1.68s | 389.55 MB/s | 0.97s | 678.99 MB/s |
| `tar + zstd (-T0 -3)` | Auto | Stream | 473.40 MB | 72.24% | 1.05s | 622.10 MB/s | 0.74s | 884.05 MB/s |
| `tar + pigz (-6)` | Auto | Stream | 462.09 MB | 70.51% | 3.40s | 193.00 MB/s | 2.35s | 278.92 MB/s |

### Key Takeaways

* **Beats `tar + zstd` at multi-GB scale:** On the 5.33 GB dataset, `PulsarV2-zstd` reaches **532.41 MB/s** compression and **590.23 MB/s** extraction, outperforming `tar + zstd` (`506.87 MB/s` comp, `521.16 MB/s` ext) while maintaining indexed random-access chunks.
* **2.2x–2.9x faster than `tar + pigz`:** Both Zstd and DEFLATE modes substantially outperform `tar + pigz` across compression and parallel extraction while matching archive size within `0.13%` (`8192 KB` Zstd).
* **Codec-aware scaling:** Zstd peaks at **4–6 workers** (physical core saturation without L3 cache contention), while DEFLATE scales linearly up to **12 workers**.

### Running Benchmarks

Requires `zip`, `7z`, `tar`, `pigz`, and `zstd` for the full comparison. Missing tools are automatically skipped.

```bash
go build -o ./plsr ./cmd

mkdir -p ~/benchmark-tmp

TMPDIR="$HOME/benchmark-tmp" go run ./benchmarks \
    -plsr ./plsr \
    -input ./testdata5GB \
    -runs 3
```

---

## Archive Format

### V1 (File-Level)

```text
+----------+-------------+-------------------+----------+
|  Header  |  File Data  | Central Directory |  Footer  |
+----------+-------------+-------------------+----------+
```

### V2 (Chunk-Based)

```text
+----------+--------------+--------------------------+----------+
|  Header  |  Chunk Data  | File Table + Chunk Index |  Footer  |
+----------+--------------+--------------------------+----------+
```

In **V2**, `--chunk-size 0` dynamically scales each file's chunk size between `64 KB` and `4096 KB` (snapped to powers of 2 via `fileSize / (workers * 2)`), keeping small files unsplit while saturating worker threads on large files. The central directory stores filenames, byte offsets, original/compressed sizes, compression methods, and CRC32 checksums, allowing metadata inspection without scanning payloads and enabling parallel chunk-level decompression.

---

## Project Structure

```text
Pulsar/
├── cmd/            # CLI entry point and subcommands (create, extract, list, info)
├── internal/       # Core archiver: V1/V2 writers, reader, chunker, sync.Pool, DEFLATE, CRC32
├── benchmarks/     # Comparative benchmark harness
├── go.mod          # Go module definition (Go 1.27.1)
└── LICENSE         # MIT License
```

---

## Development Roadmap

- [x] Implement chunk-based parallel compression and extraction (V2).
- [x] Implement adaptive per-file dynamic chunking and power-of-2 `sync.Pool` buffer recycling.
- [x] Add Zstandard (`zstd`) support.
- [ ] Improve extraction safety and partial/single-file extraction.
- [ ] Package for Arch Linux (AUR) and add desktop file-manager integration.

---

## License

Licensed under the [MIT License](LICENSE).
