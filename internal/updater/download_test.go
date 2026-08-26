package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchRetriesAfterServerError(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "asset")
	d := newDownloader()
	d.retryDelay = 0
	if err := d.fetch(context.Background(), server.URL, path); err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("content = %q, want %q", got, "payload")
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2 (one failure then one retry)", hits)
	}
}

func TestFetchResumesWithRangeHeaderInsteadOfRestarting(t *testing.T) {
	const full = "0123456789"
	var sawRange atomic.Value
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte(full[:4]))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			panic(http.ErrAbortHandler) // 中途断开，留下半截文件
		}
		sawRange.Store(r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes 4-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(full[4:]))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "asset")
	d := newDownloader()
	d.retryDelay = 0
	if err := d.fetch(context.Background(), server.URL, path); err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != full {
		t.Fatalf("content = %q, want %q", got, full)
	}
	if r, _ := sawRange.Load().(string); r != "bytes=4-" {
		t.Fatalf("Range header = %q, want %q — 续传未生效，重试是从头开始的", r, "bytes=4-")
	}
}

func TestFetchGivesUpWhenTransferStalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("x"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(2 * time.Second) // 之后一个字节都不再发
	}))
	defer server.Close()

	d := newDownloader()
	d.attempts = 1
	d.retryDelay = 0
	d.stallWindow = 150 * time.Millisecond

	start := time.Now()
	err := d.fetch(context.Background(), server.URL, filepath.Join(t.TempDir(), "asset"))
	if err == nil {
		t.Fatalf("fetch() error = nil, want a stall error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("fetch() took %v, want it to abort near the %v stall window", elapsed, d.stallWindow)
	}
}

func TestFetchDoesNotRetryOnNotFound(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	d := newDownloader()
	d.retryDelay = 0
	err := d.fetch(context.Background(), server.URL, filepath.Join(t.TempDir(), "asset"))
	if err == nil {
		t.Fatalf("fetch() error = nil, want a 404 error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %v, want it to mention 404", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1 — 404 是确定性失败，不应重试", hits)
	}
}

func TestBytesReadsSmallPayloadIntoMemory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("checksums"))
	}))
	defer server.Close()

	d := newDownloader()
	d.retryDelay = 0
	got, err := d.bytes(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("bytes() error = %v", err)
	}
	if string(got) != "checksums" {
		t.Fatalf("bytes() = %q, want %q", got, "checksums")
	}
}
