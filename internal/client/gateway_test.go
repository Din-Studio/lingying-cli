package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListModelsUsesConfiguredGatewayBaseAndBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer server.Close()

	c := NewWithBaseURL("secret", server.URL)
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("models = %d, want 0", len(models))
	}
}

func TestUploadFileStreamsMultipartAndChecksHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/files" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		got, err := io.ReadAll(file)
		if err != nil || !bytes.Equal(got, []byte("hello")) {
			t.Fatalf("uploaded = %q, err=%v", got, err)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"download_url":"https://files.test/hello"}}`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	url, err := NewWithEndpoints("secret", "https://gateway.test", server.URL).UploadFile(context.Background(), "hello.txt", path)
	if err != nil || url != "https://files.test/hello" {
		t.Fatalf("UploadFile() = %q, %v", url, err)
	}
}

func TestDownloadPreservesDestinationOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "result.bin")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	err := New("secret").Download(context.Background(), server.URL, path)
	if err == nil {
		t.Fatal("Download() error = nil, want HTTP failure")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "old" {
		t.Fatalf("destination = %q, err=%v", got, readErr)
	}
}

func TestExtractResultURLsFindsNestedMediaURLs(t *testing.T) {
	urls, err := ExtractResultURLs([]byte(`{"data":{"result":{"files":[{"url":"https://example.test/a.png"}],"nested":{"download_url":"https://example.test/b.png"}}}}`))
	if err != nil {
		t.Fatalf("ExtractResultURLs() error = %v", err)
	}
	if len(urls) != 2 || urls[0] != "https://example.test/a.png" || urls[1] != "https://example.test/b.png" {
		t.Fatalf("urls = %#v", urls)
	}
}
