# Pulsar

**A custom lossless file archiver written in Go.**

Pulsar is a command-line archiving utility built around a custom binary archive format. It supports DEFLATE compression, concurrent file processing, directory archiving, and parallel extraction.

The project explores low-level file I/O, binary serialization, compression pipelines, concurrency, and filesystem operations in Go.

## Features

- **Custom Archive Format** — Headers, file metadata, central directory, and footer.
- **Lossless Compression** — DEFLATE compression with configurable compression levels.
- **Concurrent Processing** — Worker-pool-based parallel compression and decompression.
- **Directory Support** — Nested directories, files, and empty directories.
- **Integrity Checking** — CRC32 checksums for detecting corrupted file contents.
- **Archive Listing** — Hierarchical tree representation of archive contents.
- **Standalone CLI** — Lightweight command-line interface.

## Installation

```bash
git clone https://github.com/Tanishq-Mehta-1/pulsar.git
cd pulsar
go build -o plsr ./cmd/pulsar
```

Optionally, install system-wide:

```bash
sudo install plsr /usr/local/bin/plsr
```

## Usage

### Create an archive

```bash
plsr create backup.fz ./project
```

Multiple inputs:

```bash
plsr create backup.fz ./project ./notes.txt ./image.png
```

Specify compression workers:

```bash
plsr create backup.fz ./project --workers 8
```

Specify output directory:

```bash
plsr create backup.fz ./project -o ./archives
```

### Extract an archive

```bash
plsr extract backup.fz
```

Extract to a specific directory:

```bash
plsr extract backup.fz -o ./output
```

Specify decompression workers:

```bash
plsr extract backup.fz --workers 8
```

### List archive contents

```bash
plsr list backup.fz
```

Example output:

```text
project/
├── src/
│   ├── main.go
│   └── utils.go
├── assets/
│   └── logo.png
├── config.json
└── README.md
```

## Archive Format

Pulsar uses a custom binary archive layout:

```text
┌──────────────────────┐
│        Header        │
├──────────────────────┤
│                      │
│      File Data       │
│                      │
├──────────────────────┤
│    Central Directory │
├──────────────────────┤
│        Footer        │
└──────────────────────┘
```

- **Header:** Identifies the archive and stores format metadata.
- **File Data:** Contains compressed file payloads.
- **Central Directory:** Stores filenames, offsets, sizes, compression methods, and checksums.
- **Footer:** Contains information required to locate and parse the central directory.

The central directory allows the reader to locate archive metadata without scanning every file payload.

## Technical Details

| Component | Implementation |
|---|---|
| Language | Go |
| Compression | `compress/flate` |
| Integrity | CRC32 (IEEE) |
| Concurrency | Goroutines and worker pools |
| File I/O | `os`, `io`, `io.SectionReader` |
| Archive format | Custom binary format |

## Project Structure

```text
pulsar/
├── cmd/
│   └── pulsar/
│       ├── main.go
│       ├── create.go
│       ├── extract.go
│       └── list.go
├── internal/
│   └── archive/
│       ├── writer.go
│       ├── reader.go
│       ├── format.go
│       ├── compress.go
│       ├── decompress.go
│       └── worker.go
├── go.mod
└── README.md
```

## License

Pulsar is licensed under the [MIT License](LICENSE).