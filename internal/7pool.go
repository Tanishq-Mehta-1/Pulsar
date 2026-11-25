package archive

import (
	"math/bits"
	"sync"
)

//Sync pool with 8 pools, one each for 64,128,256,512,1024,2048,4096 KB

const (
	//bit shift
	minPoolShift = 16
	maxPoolShift = 22
	numPoolSizes = maxPoolShift - minPoolShift + 1
)

var chunkPools [numPoolSizes]sync.Pool

func init() {
	for i := range numPoolSizes {
		size := 1 << (minPoolShift + i)

		//if we fetch this size,
		chunkPools[i].New = func() any {
			b := make([]byte, size)
			return &b
		}
	}
}

func getChunkBuffer(size int) []byte {
	if size <= 0 {
		return nil
	}

	if size > (1 << maxPoolShift) {
		return make([]byte, size)
	}

	idx := 0
	if size > (1 << minPoolShift) {
		idx = bits.Len(uint(size-1)) - minPoolShift
	}

	bufPtr := chunkPools[idx].Get().(*[]byte)
	return (*bufPtr)[:size] //just enough, no more
}

func putChunkBuffer(buf []byte) {

	c := cap(buf)
	if c < (1<<minPoolShift) || c > (1<<maxPoolShift) {
		return
	}
	// Only recycle exact power-of-2 capacities that belong to our buckets.
	if c&(c-1) != 0 {
		return
	}

	idx := bits.TrailingZeros(uint(c)) - minPoolShift
	b := buf[:c]
	chunkPools[idx].Put(&b)

}
