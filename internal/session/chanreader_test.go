package session

import (
	"errors"
	"io"
	"sync"
	"testing"
)

func TestChanReaderBasic(t *testing.T) {
	cr := newChanReader(0)

	// Send 3 chunks, then close.
	chunks := []string{"hello", " ", "world"}
	for _, c := range chunks {
		cr.ch <- []byte(c)
	}
	cr.close()

	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestChanReaderPartialRead(t *testing.T) {
	cr := newChanReader(0)

	cr.ch <- []byte("ABCDEFGH")
	cr.close()

	// Read in 3-byte pieces.
	buf := make([]byte, 3)
	var got []byte
	for {
		n, err := cr.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("Read: %v", err)
		}
	}
	if string(got) != "ABCDEFGH" {
		t.Fatalf("got %q, want %q", got, "ABCDEFGH")
	}
}

func TestChanReaderCloseWithError(t *testing.T) {
	cr := newChanReader(0)
	myErr := errors.New("connection lost")

	// Buffer some data then close with error.
	cr.ch <- []byte("partial")
	cr.closeWithError(myErr)

	// Should get buffered data first.
	buf := make([]byte, 100)
	n, err := cr.Read(buf)
	if err != nil {
		t.Fatalf("first Read should succeed, got err: %v", err)
	}
	if string(buf[:n]) != "partial" {
		t.Fatalf("first Read: got %q, want %q", buf[:n], "partial")
	}

	// Next read should return the error.
	_, err = cr.Read(buf)
	if !errors.Is(err, myErr) {
		t.Fatalf("second Read: got %v, want %v", err, myErr)
	}
}

func TestChanReaderConcurrent(t *testing.T) {
	cr := newChanReader(0)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cr.ch <- []byte("x")
		}
		cr.close()
	}()

	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("got %d bytes, want 100", len(got))
	}
	wg.Wait()
}

func TestChanReaderCloseBeforeRead(t *testing.T) {
	cr := newChanReader(0)
	cr.close()

	buf := make([]byte, 10)
	n, err := cr.Read(buf)
	if n != 0 {
		t.Fatalf("expected 0 bytes, got %d", n)
	}
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestChanReaderDoubleClose(t *testing.T) {
	cr := newChanReader(0)
	cr.close()
	cr.close() // must not panic
}

func TestChanReaderDoubleCloseWithError(t *testing.T) {
	cr := newChanReader(0)
	cr.closeWithError(errors.New("err1"))
	cr.closeWithError(errors.New("err2")) // must not panic; first error wins

	buf := make([]byte, 10)
	_, err := cr.Read(buf)
	if err == nil || err.Error() != "err1" {
		t.Fatalf("expected err1, got %v", err)
	}
}
