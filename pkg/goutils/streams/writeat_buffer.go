package streams

import (
	"fmt"
	"io"
	"sort"
)

// A buffer that implements the io.WriteAt
// interface and writing data to a internal
// buffer which get's written to the output
// when a contiguous set of bytes become
// available.
type WriteAtBuffer struct {
	data       io.Writer
	outAt      int64
	bufferSlot []bufferAt
}

type bufferAt struct {
	pos  int64
	size int
	data []byte
}

func NewWriteAtBuffer(data io.Writer) *WriteAtBuffer {
	return &WriteAtBuffer{
		data:       data,
		outAt:      0,
		bufferSlot: []bufferAt{},
	}
}

func (w *WriteAtBuffer) Close() error {
	if len(w.bufferSlot) != 0 {
		return fmt.Errorf("buffer has unwritten data: %# v", w.bufferSlot)
	}
	return nil
}

func (w *WriteAtBuffer) WriteAt(b []byte, offset int64) (int, error) {

	var (
		err  error
		n, i int
		s    bufferAt
	)

	retSize := len(b)
	outSize := retSize

	if offset == w.outAt {
		if n, err = w.data.Write(b); err != nil {
			return n, err
		}
		if outSize > n {
			// buffer only partially written so
			// remainder needs to be saved in a slot
			b = b[n:]
			offset += int64(n)
			outSize -= n
			retSize = n
		} else {
			outSize = 0
		}
		w.outAt += int64(n)
	}

	if outSize > 0 {

		// sort the slots in order of write position
		numSlots := len(w.bufferSlot)
		if numSlots > 0 {
			index := sort.Search(numSlots,
				func(i int) bool {
					return offset < w.bufferSlot[i].pos
				})

			w.bufferSlot = append(w.bufferSlot, bufferAt{})
			copy(w.bufferSlot[index+1:], w.bufferSlot[index:])
			w.bufferSlot[index] = bufferAt{
				pos:  offset,
				size: outSize,
				data: b,
			}
		} else {
			w.bufferSlot = append(w.bufferSlot, bufferAt{
				pos:  offset,
				size: outSize,
				data: b,
			})
		}
	}

	// cycle through the available slots to determine
	// if contiguous slots are available to be written
	last := len(w.bufferSlot) - 1
	for i, s = range w.bufferSlot {
		if w.outAt > s.pos {
			return retSize,
				fmt.Errorf(
					"data pointer at %d but need to write bufferred data from %d",
					w.outAt, s.pos)
		} else if w.outAt < s.pos {
			break
		}

		if n, err = w.data.Write(s.data); err != nil {
			return retSize, err
		}
		w.outAt += int64(n)
		if i == last {
			w.bufferSlot = []bufferAt{}
		} else {
			w.bufferSlot = w.bufferSlot[1:]
		}
	}

	return retSize, nil
}
