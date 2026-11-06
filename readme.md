# Pulsar

**A custom lossless file archiver written in Go.**

Pulsar is a command-line archiving utility built around a custom binary archive format, featuring concurrent compression, parallel extraction, directory support, and integrity verification.

## Features

- Custom binary archive format with a central directory.
- DEFLATE compression with configurable levels.
- Concurrent compression and parallel extraction.
- Nested directories and empty directory support.
- CRC32 integrity verification.
- Archive listing and metadata inspection.
- Benchmarking against established compression tools.

## Installation

```bash
git clone https://github.com/Tanishq-Mehta-1/pulsar.git
cd pulsar
go build -o plsr .
```

Optionally, install system-wide:

```bash
sudo install plsr /usr/local/bin/plsr
```

## Usage

```bash
# Create an archive
plsr create backup.fz ./project

# Multiple inputs and worker configuration
plsr create backup.fz ./project ./notes.txt --workers 8

# Specify output directory
plsr create backup.fz ./project -o ./archives

# Extract
plsr extract backup.fz -o ./output --workers 8

# List contents
plsr list backup.fz

# Display archive metadata
plsr info backup.fz
```

## Benchmarks

Tested on a **5.33 GB dataset**, with median results over 3 runs.

| Implementation | Workers | Archive (MB) | Compress (s) | Extract (s) |
|---|---:|---:|---:|---:|
| Pulsar | 1 | 3352.25 | 92.44 | 35.47 |
| Pulsar | 2 | 3352.25 | 53.71 | 22.13 |
| Pulsar | 4 | 3352.25 | 38.94 | 22.07 |
| Pulsar | 8 | 3352.25 | 36.44 | 19.29 |
| Pulsar | 16 | 3352.25 | 36.19 | 17.30 |
| Pulsar | 64 | 3352.25 | 37.44 | 16.97 |
| System ZIP | 1 | 3244.12 | 202.41 | 38.11 |
| 7z Deflate | 1 | 3142.56 | 230.18 | 33.12 |
| 7z LZMA2 | 1 | 2045.27 | 575.79 | 11.88 |
| tar + pigz | Auto | 3246.94 | 36.36 | 31.91 |
| tar + zstd | Auto | 3186.82 | 10.30 | 11.43 |

Go ZIP was also tested as an additional baseline.

*Results depend on compression settings, threading, and hardware. These implementations do not all use equivalent configurations.*

### Running benchmarks

Requires `zip`, `7z`, `tar`, `pigz`, and `zstd` for the full comparison. Missing tools are skipped.

```bash
go build -o plsr .

mkdir -p ~/benchmark-tmp

TMPDIR="$HOME/benchmark-tmp" go run ./benchmarks \
    -plsr ./plsr \
    -input ./testdata \
    -runs 3
```

## Archive Format

```text
[ Header ][ File Data ][ Central Directory ][ Footer ]
```

The central directory stores filenames, offsets, sizes, compression methods, and checksums, allowing archive metadata to be located without scanning file payloads.

## Development Roadmap

- Profile and optimize compression and I/O performance.
- Add Zstandard support.
- Implement chunk-based parallel compression for large files.
- Improve memory efficiency and extraction safety.
- Package for Arch Linux (AUR) and add desktop file-manager integration.

## Tech Stack

Go · DEFLATE · Goroutines · Worker Pools · CRC32 · Binary Serialization

## License

[MIT License](LICENSE)ye 