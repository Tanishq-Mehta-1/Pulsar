# Pulsar

**A custom lossless file archiver written in Go.**

Pulsar is a command-line archiving utility built around a custom binary archive format (V1 file-level and V2 chunk-based), featuring concurrent compression, parallel extraction, directory support, and integrity verification.

## Features

- Dual-version binary archive format with a central directory:
  - **V1:** File-level parallel compression and extraction.
  - **V2:** Chunk-based parallel compression and extraction with configurable chunk sizes, enabling multi-core scaling even on single large files and indexed random access.
- DEFLATE compression with configurable levels.
- Nested directories and empty directory support.
- Per-file / per-chunk CRC32 integrity verification.
- Archive listing and metadata inspection (`list`, `info`).
- Built-in benchmarking harness against established compression tools.

## Installation

```bash
git clone https://github.com/Tanishq-Mehta-1/pulsar.git
cd pulsar
go build -o ./plsr ./cmd
```

Optionally, install system-wide:

```bash
sudo install plsr /usr/local/bin/plsr
```

## Usage

```bash
# Create an archive (default v1)
plsr create backup ./project

# Create a V2 chunk-based archive with custom chunk size (in KB)
plsr create backup ./project -v 2 --chunk-size 256 --workers 8

# Multiple inputs and output directory
plsr create backup ./project ./notes.txt -o ./archives --workers 8

# Extract (auto-detects V1/V2)
plsr extract backup.plsr -o ./output --workers 8

# List contents
plsr list backup.plsr

# Display archive metadata
plsr info backup.plsr
```

## Benchmarks

Tested on a **655.35 MB dataset**, with median results over 3 runs (Pulsar V2 configured with a **256 KB** chunk size).

| Implementation | Workers | Archive (MB) | Compress (s) | Extract (s) | Ratio (%) | Comp (MB/s) | Extract (MB/s) |
|---|---:|---:|---:|---:|---:|---:|---:|
| Pulsar V1 | 2 | 478.66 | 7.37 | 2.89 | 73.04 | 88.98 | 227.12 |
| Pulsar V1 | 4 | 478.66 | 5.26 | 2.49 | 73.04 | 124.50 | 262.93 |
| Pulsar V1 | 8 | 478.66 | 5.02 | 2.26 | 73.04 | 130.65 | 289.85 |
| Pulsar V1 | 16 | 478.66 | 5.20 | 2.15 | 73.04 | 126.00 | 304.29 |
| Pulsar V1 | 64 | 478.66 | 5.25 | 2.15 | 73.04 | 124.81 | 304.42 |
| Pulsar V2 | 1 | 481.64 | 9.15 | 4.81 | 73.49 | 71.65 | 136.11 |
| Pulsar V2 | 2 | 481.64 | 5.00 | 2.99 | 73.49 | 131.13 | 219.19 |
| Pulsar V2 | 4 | 481.64 | 2.98 | 2.01 | 73.49 | 219.88 | 325.84 |
| Pulsar V2 | 8 | 481.64 | 2.24 | 1.83 | 73.49 | 292.20 | 358.42 |
| Pulsar V2 | 16 | 481.64 | 2.07 | 1.95 | 73.49 | **316.49** | 336.85 |
| Pulsar V2 | 64 | 481.64 | 2.23 | 1.60 | 73.49 | 293.59 | **409.49** |
| Go ZIP | 1 | 480.30 | 8.78 | 4.59 | 73.29 | 74.67 | 142.69 |
| tar + pigz | Auto | 462.09 | 3.62 | 2.82 | 70.51 | 180.83 | 232.27 |
| tar + zstd | Auto | 473.40 | 1.25 | 1.36 | 72.24 | 526.00 | 480.68 |

*Results depend on compression settings, threading, and hardware. These implementations do not all use equivalent configurations.*

### Running benchmarks

Requires `zip`, `7z`, `tar`, `pigz`, and `zstd` for the full comparison. Missing tools are skipped.

```bash
go build -o ./plsr ./cmd

mkdir -p ~/benchmark-tmp

TMPDIR="$HOME/benchmark-tmp" go run ./benchmarks \
    -plsr ./plsr \
    -input ./testdata \
    -runs 3 \
    -chunk-size 256
```

## Archive Format

### V1 (File-Level)

```text
[ Header ][ File Data ][ Central Directory ][ Footer ]
```

### V2 (Chunk-Based)

```text
[ Header ][ Chunk Data ][ File Table + Chunk Index ][ Footer ]
```

The central directory stores filenames, byte offsets, original/compressed sizes, compression methods, and CRC32 checksums, allowing archive metadata to be located without scanning payloads and enabling parallel chunk-level decompression.

## Development Roadmap

- [x] Implement chunk-based parallel compression and extraction (V2).
- [ ] Profile and optimize memory/buffer pooling (`sync.Pool`) and I/O throughput.
- [ ] Add Zstandard (`zstd`) support.
- [ ] Improve extraction safety and partial/single-file extraction.
- [ ] Package for Arch Linux (AUR) and add desktop file-manager integration.

## Tech Stack

Go · DEFLATE · Goroutines · Worker Pools · CRC32 · Binary Serialization

## License

[MIT License](LICENSE)