package archive

import (
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	CodecStored  uint8 = 0
	CodecDeflate uint8 = 1
	CodecZstd    uint8 = 2
)

func ParseCodec(s string) (uint8, error) {
	switch s {
	case "deflate":
		return CodecDeflate, nil
	case "zstd":
		return CodecZstd, nil
	case "stored":
		return CodecStored, nil
	default:
		return 255, fmt.Errorf("CODEC::invalid codec option: %s", s)
	}
}

var zstdEncoder = sync.Pool{
	New: func() any {
		writer, _ := zstd.NewWriter(nil,
			zstd.WithEncoderConcurrency(1),
			zstd.WithEncoderLevel(zstd.SpeedDefault),
			zstd.WithEncoderCRC(false),
		)
		return writer
	},
}

var zstdDecoder = sync.Pool{
	New: func() any {
		decoder, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(false))
		return decoder
	},
}

func CompressZstd(src []byte, dst []byte) []byte {

	enc := zstdEncoder.Get().(*zstd.Encoder)
	defer zstdEncoder.Put(enc)

	return enc.EncodeAll(src, dst[:0])
}

func DecompressZstd(src []byte, dst []byte) ([]byte, error) {
	dec := zstdDecoder.Get().(*zstd.Decoder)
	defer zstdDecoder.Put(dec)

	return dec.DecodeAll(src, dst[:0])
}

func CompressZstdStream(dst io.Writer, src io.Reader, hasher io.Writer) (int64, error) {
	enc := zstdEncoder.Get().(*zstd.Encoder)
	enc.Reset(dst)
	defer func() {
		enc.Reset(nil)
		zstdEncoder.Put(enc)
	}()

	mw := io.MultiWriter(hasher, enc)
	n, err := io.Copy(mw, src)
	if err != nil {
		return n, err
	}
	if err := enc.Close(); err != nil {
		return n, err
	}
	return n, nil
}

func DecompressZstdStream(dst io.Writer, src io.Reader) (int64, error) {
	dec := zstdDecoder.Get().(*zstd.Decoder)
	if err := dec.Reset(src); err != nil {
		zstdDecoder.Put(dec)
		return 0, err
	}
	defer func() {
		_ = dec.Reset(nil)
		zstdDecoder.Put(dec)
	}()

	return io.Copy(dst, dec)
}
