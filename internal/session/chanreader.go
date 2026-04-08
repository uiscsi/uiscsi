package session

import (
	"io"
	"sync"
)

// defaultChanBufSize is the default channel buffer capacity for streaming
// Data-In delivery. Configurable via WithStreamBufDepth.
const defaultChanBufSize = 128

// chanReader streams data from a producer (handleDataIn) to a consumer
// (the caller reading Result.Data or the io.Reader from SubmitStreaming)
// via a buffered channel. Memory is bounded to chanBufSize chunks
// regardless of total transfer size.
//
// The producer sends []byte slices via the ch channel. These slices are
// caller-owned (allocated in framer.go ReadRawPDU, not pooled) and safe
// to retain. The consumer reads via Read(). When the producer calls
// close(), the reader returns io.EOF. On error, closeWithError(err)
// causes Read() to return that error after all buffered data is drained.
type chanReader struct {
	ch      chan []byte // incoming data chunks from handleDataIn
	current []byte     // partially consumed chunk from the last Read call
	mu      sync.Mutex // protects err and closed
	err     error      // terminal error, set before closing ch
	closed  bool       // guards against double close
}

// newChanReader creates a chanReader with the given buffer depth.
// If depth <= 0, defaultChanBufSize is used.
func newChanReader(depth int) *chanReader {
	if depth <= 0 {
		depth = defaultChanBufSize
	}
	return &chanReader{
		ch: make(chan []byte, depth),
	}
}

// Read implements io.Reader. It returns data from the buffered channel,
// blocking if no data is available yet. Returns io.EOF when the channel
// is closed and all buffered data has been consumed. Returns the producer's
// error (from closeWithError) after draining all buffered data.
func (cr *chanReader) Read(p []byte) (int, error) {
	// Drain any leftover bytes from the previous chunk first.
	if len(cr.current) > 0 {
		n := copy(p, cr.current)
		cr.current = cr.current[n:]
		return n, nil
	}

	// Block until the next chunk arrives or the channel is closed.
	chunk, ok := <-cr.ch
	if !ok {
		// Channel closed — return the terminal error or EOF.
		cr.mu.Lock()
		err := cr.err
		cr.mu.Unlock()
		if err != nil {
			return 0, err
		}
		return 0, io.EOF
	}

	n := copy(p, chunk)
	if n < len(chunk) {
		cr.current = chunk[n:]
	}
	return n, nil
}

// close signals the reader that no more data will arrive. Subsequent
// Read calls will return io.EOF after all buffered data is consumed.
func (cr *chanReader) close() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.closed {
		return
	}
	cr.closed = true
	close(cr.ch)
}

// closeWithError signals the reader with a specific error. After all
// buffered data is consumed, Read will return err instead of io.EOF.
func (cr *chanReader) closeWithError(err error) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.closed {
		return
	}
	cr.err = err
	cr.closed = true
	close(cr.ch)
}
