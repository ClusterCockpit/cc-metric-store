package memstore

import (
	"errors"
	"log"
	"os"
	"reflect"
	"sync"
	"unsafe"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
	"golang.org/x/sys/unix"
)

const bufferSizeInFloats int = 512
const bufferSizeInBytes int = bufferSizeInFloats * 8

// The allocator rarely used, so a single big lock should be fine!
var allocatorLock sync.Mutex
var allocatorPool [][]byte
var allocatorBatch []byte

func RequestBytes(size int) []byte {
	requested := size
	size = (size + bufferSizeInBytes - 1) / bufferSizeInBytes * bufferSizeInBytes

	// Check allocation caches:
	allocatorLock.Lock()
	if len(allocatorPool) > 0 && size == bufferSizeInBytes {
		bytes := allocatorPool[len(allocatorPool)-1]
		allocatorPool = allocatorPool[:len(allocatorPool)-1]
		allocatorLock.Unlock()
		return bytes[:requested]
	} else if cap(allocatorBatch) > size {
		bytes := allocatorBatch[:0:size]
		allocatorBatch = allocatorBatch[size:]
		allocatorLock.Unlock()
		return bytes[:requested]
	}
	allocatorLock.Unlock()

	pagesize := os.Getpagesize()
	if size < pagesize || size%pagesize != 0 {
		panic("page size and buffer size do not go with each other")
	}

	bytes, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_SHARED)
	if err != nil {
		panic("unix.Mmap failed: " + err.Error())
	}

	if cap(bytes) != size {
		panic("whoops?")
	}

	return bytes[:requested]
}

func FillAllocatorCache(estimate int) error {
	size := (estimate + bufferSizeInBytes - 1) / bufferSizeInBytes * bufferSizeInBytes
	pagesize := os.Getpagesize()
	if size < pagesize || size%pagesize != 0 {
		return errors.New("estimate to small of buffer size not page size compatible")
	}

	allocatorLock.Lock()
	defer allocatorLock.Unlock()

	if len(allocatorBatch) > 0 {
		n := len(allocatorBatch) / bufferSizeInBytes
		for i := 0; i < n; i++ {
			chunk := allocatorBatch[i*bufferSizeInBytes : i*bufferSizeInBytes+bufferSizeInBytes]
			allocatorPool = append(allocatorPool, chunk[0:0:bufferSizeInBytes])
		}
		allocatorBatch = nil
	}

	bytes, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_SHARED)
	if err != nil {
		panic("unix.Mmap failed: " + err.Error())
	}

	log.Printf("batch-allocated %d bytes", cap(bytes))
	allocatorBatch = bytes
	return nil
}

func ReleaseBytes(bytes []byte) {
	if cap(bytes)%bufferSizeInBytes != 0 {
		panic("bytes released that do not match the buffer size constraints")
	}

	allocatorLock.Lock()
	defer allocatorLock.Unlock()

	n := cap(bytes) / bufferSizeInBytes
	for i := 0; i < n; i++ {
		chunk := bytes[i*bufferSizeInBytes : i*bufferSizeInBytes+bufferSizeInBytes]
		allocatorPool = append(allocatorPool, chunk[0:0:bufferSizeInBytes])
	}
}

func ReleaseFloatSlice(slice []types.Float) {
	var x types.Float
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(sh.Data)), sh.Cap*int(unsafe.Sizeof(x)))
	ReleaseBytes(bytes)
}

func RequestFloatSlice(size int) []types.Float {
	var x types.Float
	bytes := RequestBytes(size * int(unsafe.Sizeof(x)))
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
	return unsafe.Slice((*types.Float)(unsafe.Pointer(sh.Data)), size)
}
