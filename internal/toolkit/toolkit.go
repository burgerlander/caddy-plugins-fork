// Package toolkit contains useful, general-purpose utilities
package toolkit

import (
	"bytes"
	"sync"
)

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// GetBuffer returns an empty buffer, along with a function which can be used to
// return it to a global pool, which helps reduce allocations.
func GetBuffer() (*bytes.Buffer, func()) {
	buf := bufPool.Get().(*bytes.Buffer)
	return buf, func() {
		buf.Reset()
		bufPool.Put(buf)
	}
}
