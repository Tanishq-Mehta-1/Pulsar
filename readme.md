# Pulsar

**A custom lossless file archiver written in Go.**

Pulsar is a command-line archiving utility built around a custom binary archive format (V1 file-level and V2 chunk-based), featuring concurrent compression, parallel extraction, adaptive per-file dynamic chunking, directory support, and integrity verification.

## Features

* Dual-version binary archive format with a central directory:

  * **V1:** File-level parallel compression and extraction.

  * **V2:** Chunk-based parallel compression and extraction with adaptive per-file dynamic chunking (`--chunk-size 0`, default) or fixed chunk sizes, enabling multi-core scaling even on single large files and indexed random access.

* Power-of-2 buffer pooling (`64 KB`–`4 MB` `sync.Pool` buckets) to minimize GC overhead and peak heap allocations during chunked I/O.

* DEFLATE compression with configurable levels.

* Nested directories and empty directory support.

* Per-file / per-chunk CRC32 integrity verification.

* Archive listing and metadata inspection (`list`, `info`).

* Built-in benchmarking harness against established compression tools.

## Installation

```
git clone https://github.com/Tanishq-Mehta-1/pulsar.git
cd pulsar
go build -o ./plsr ./cmd

```

Optionally, install system-wide:

```
sudo install plsr /usr/local/bin/plsr

```

## Usage

```
# Create an archive (default v1)
plsr create backup ./project

# Create a V2 chunk-based archive with adaptive per-file chunk sizing (default: --chunk-size 0)
plsr create backup ./project -v 2 --workers 8

# Create a V2 chunk-based archive with a fixed chunk size (in KB)
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

Tested on a **10.01 GB dataset (10,005.03 MB)**, with median results over 3 runs (Pulsar V2 configured with a **256 KB** chunk size).

| Implementation | Workers | Archive (MB) | Compress (s) | Extract (s) | Ratio (%) | Comp (MB/s) | Extract (MB/s) | 
| ----- | ----- | ----- | ----- | ----- | ----- | ----- | ----- | 
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

*Results depend on compression settings, threading, and hardware. These implementations do not all use equivalent configurations.*

### Running benchmarks

Requires `zip`, `7z`, `tar`, `pigz`, and `zstd` for the full comparison. Missing tools are skipped.

```
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

```
[ Header ][ File Data ][ Central Directory ][ Footer ]

```

### V2 (Chunk-Based)

```
[ Header ][ Chunk Data ][ File Table + Chunk Index ][ Footer ]

```

In V2, `--chunk-size 0` (the default) dynamically scales each file's chunk size between `64 KB` and `4096 KB` (snapped to powers of 2 via `fileSize / (workers * 2)`), keeping small files unsplit while saturating worker threads on large files. The central directory stores filenames, byte offsets, original/compressed sizes, compression methods, and CRC32 checksums, allowing archive metadata to be located without scanning payloads and enabling parallel chunk-level decompression.

## Development Roadmap

* \[x\] Implement chunk-based parallel compression and extraction (V2).

* \[x\] Implement adaptive per-file dynamic chunking and power-of-2 `sync.Pool` buffer recycling.

* \[ \] Add Zstandard (`zstd`) support.

* \[ \] Improve extraction safety and partial/single-file extraction.

* \[ \] Package for Arch Linux (AUR) and add desktop file-manager integration.

## Tech Stack

Go · DEFLATE · Goroutines · Worker Pools · `sync.Pool` · CRC32 · Binary Serialization

## License

[MIT License](LICENSE)
