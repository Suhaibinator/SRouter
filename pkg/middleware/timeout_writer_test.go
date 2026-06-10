package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestTimeoutDoesNotOverwriteStartedResponse verifies that when the handler
// has already started writing a response before the timeout fires, the
// middleware does not write a 408 on top of it: the client receives the
// handler's partial response untouched.
func TestTimeoutDoesNotOverwriteStartedResponse(t *testing.T) {
	wrote := make(chan struct{})
	release := make(chan struct{})
	handlerDone := make(chan struct{})

	handler := timeout(30 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		close(wrote)
		<-release // Keep running past the timeout.
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	// The middleware returned at the timeout. Synchronize with the handler's
	// write, after which it stays blocked, so the recorder is safe to inspect.
	<-wrote
	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d (handler's status must not be replaced by a timeout response)", rec.Code, http.StatusAccepted)
	}
	if got := rec.Body.String(); got != "partial" {
		t.Errorf("body = %q, want %q (no timeout error may be appended)", got, "partial")
	}

	close(release)
	<-handlerDone
}

// TestMutexResponseWriterRejectsAllWritesAfterTimeout verifies that once the
// timeout has fired, a late handler can no longer touch the underlying
// response writer: WriteHeader is dropped, Write fails with
// http.ErrHandlerTimeout, and Flush is a no-op.
func TestMutexResponseWriterRejectsAllWritesAfterTimeout(t *testing.T) {
	rec := httptest.NewRecorder()
	var mu sync.Mutex
	rw := &mutexResponseWriter{ResponseWriter: rec, mu: &mu}
	rw.timedOut.Store(true)

	rw.WriteHeader(http.StatusTeapot)
	if rec.Code != http.StatusOK {
		t.Errorf("WriteHeader after timeout reached the underlying writer: code = %d", rec.Code)
	}
	if rw.wroteHeader.Load() {
		t.Error("WriteHeader after timeout must not mark the header as written")
	}

	n, err := rw.Write([]byte("late"))
	if n != 0 || err != http.ErrHandlerTimeout {
		t.Errorf("Write after timeout = (%d, %v), want (0, http.ErrHandlerTimeout)", n, err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("Write after timeout reached the underlying writer: body = %q", rec.Body.String())
	}

	rw.Flush()
	if rec.Flushed {
		t.Error("Flush after timeout must not flush the underlying writer")
	}
}

// TestMutexResponseWriterWriteRecheckUnderLock verifies the race window where
// a handler Write passes the initial timeout check but the timeout response is
// written while the handler is waiting for the mutex: the write must be
// rejected by the re-check under the lock instead of corrupting the response.
func TestMutexResponseWriterWriteRecheckUnderLock(t *testing.T) {
	rec := httptest.NewRecorder()
	var mu sync.Mutex
	rw := &mutexResponseWriter{ResponseWriter: rec, mu: &mu}

	// Hold the lock as the timeout path does while writing its response.
	mu.Lock()
	writeErr := make(chan error)
	go func() {
		_, err := rw.Write([]byte("late"))
		writeErr <- err
	}()

	// Let the handler write pass the initial check and block on the mutex,
	// then mark the timeout before releasing the lock.
	time.Sleep(50 * time.Millisecond)
	rw.timedOut.Store(true)
	mu.Unlock()

	if err := <-writeErr; err != http.ErrHandlerTimeout {
		t.Errorf("late Write = %v, want http.ErrHandlerTimeout", err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("late Write reached the underlying writer: body = %q", rec.Body.String())
	}
}
