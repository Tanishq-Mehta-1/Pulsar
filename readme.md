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

Tested on a **10.01 GB dataset (10,005.03 MB)**, with median results over 3 runs (Pulsar V2 configured with a **256 KB** chunk size).

| Implementation | Workers | Archive (MB) | Compress (s) | Extract (s) | Ratio (%) | Comp (MB/s) | Extract (MB/s) |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| Pulsar V1 | 1 | 6225.83 | 167.82 | 65.36 | 62.23 | 59.62 | 153.07 |
| Pulsar V1 | 2 | 6225.83 | 94.27 | 47.86 | 62.23 | 106.13 | 209.04 |
| Pulsar V1 | 4 | 6225.83 | 60.70 | 31.54 | 62.23 | 164.82 | 317.24 |
| Pulsar V1 | 8 | 6225.83 | 49.48 | 27.08 | 62.23 | 202.19 | 369.41 |
| Pulsar V1 | 16 | 6225.83 | 45.73 | 25.42 | 62.23 | 218.77 | **393.62** |
| Pulsar V1 | 64 | 6225.83 | 44.53 | 26.54 | 62.23 | 224.69 | 376.92 |
| Pulsar V2 | 1 | 6285.67 | 129.53 | 68.46 | 62.83 | 77.24 | 146.15 |
| Pulsar V2 | 2 | 6285.67 | 71.74 | 42.02 | 62.83 | 139.46 | 238.10 |
| Pulsar V2 | 4 | 6285.67 | 42.77 | 30.25 | 62.83 | 233.94 | 330.79 |
| Pulsar V2 | 8 | 6285.67 | 32.02 | 27.73 | 62.83 | **312.47** | 360.85 |
| Pulsar V2 | 16 | 6285.67 | 32.86 | 26.71 | 62.83 | 304.44 | 374.51 |
| Pulsar V2 | 64 | 6285.67 | 33.71 | 25.72 | 62.83 | 296.81 | 388.99 |
| Go ZIP | 1 | 6279.29 | 122.59 | 64.10 | 62.76 | 81.61 | 156.09 |
| System ZIP | 1 | 6026.62 | 368.87 | 68.25 | 60.24 | 27.12 | 146.60 |
| 7z Deflate | 1 | 5841.23 | 446.99 | 64.28 | 58.38 | 22.38 | 155.65 |
| 7z LZMA2 | 1 | 3772.77 | 972.84 | 21.56 | **37.71** | 10.28 | 464.01 |
| tar + pigz | Auto | 6031.77 | 55.63 | 42.22 | 60.29 | 179.84 | 236.97 |
| tar + zstd | Auto | 5897.83 | 22.40 | 20.96 | 58.95 | **446.59** | **477.34** |

> *Results depend on compression settings, threading, and hardware. These implementations do not all use equivalent configurations.*

### Running Benchmarks

Requires `zip`, `7z`, `tar`, `pigz`, and `zstd` for the full comparison. Missing tools are automatically skipped.

```bash
go build -o ./plsr ./cmd

mkdir -p ~/benchmark-tmp

TMPDIR="$HOME/benchmark-tmp" go run ./benchmarks \
    -plsr ./plsr \
    -input ./testdata \
    -runs 3 \
    -chunk-size 256
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
- [ ] Add Zstandard (`zstd`) support.
- [ ] Improve extraction safety and partial/single-file extraction.
- [ ] Package for Arch Linux (AUR) and add desktop file-manager integration.

---

## License

Licensed under the [MIT License](LICENSE).
