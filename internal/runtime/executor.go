// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"sync"
)

type boundedBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	written  int64
	maximum  int64
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := buffer.maximum - buffer.written
	if remaining <= 0 || int64(len(data)) > remaining {
		buffer.overflow = true
		return 0, ErrCommandOutputTooLarge
	}
	written, err := buffer.buffer.Write(data)
	buffer.written += int64(written)
	return written, err
}

func (buffer *boundedBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return bytes.Clone(buffer.buffer.Bytes())
}

func (buffer *boundedBuffer) Overflowed() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.overflow
}
