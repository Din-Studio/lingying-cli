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

func TestMaxUploadBytesIsOneGiB(t *testing.T) {
	const oneGiB = int64(1024 * 1024 * 1024)
	if MaxUploadBytes != oneGiB {
		t.Fatalf("MaxUploadBytes = %d, want %d", MaxUploadBytes, oneGiB)
	}
}

func TestGatewayErrorPreservesSafeUpstreamDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error_code":"INSUFFICIENT_BALANCE","message":"余额不足","request_id":"req-1"}`))
	}))
	defer server.Close()
	_, err := NewWithBaseURL("token", server.URL).ListModels(context.Background())
	details := ErrorDetails(err)
	if details.Code != "INSUFFICIENT_BALANCE" || details.Message != "余额不足" || details.RequestID != "req-1" {
		t.Fatalf("details = %#v", details)
	}
}

func TestPollTaskPreservesGatewayTaskFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"status":"failed","error_code":"INSUFFICIENT_BALANCE","error":"余额不足"}}`))
	}))
	defer server.Close()

	_, err := NewWithBaseURL("token", server.URL).PollTask(context.Background(), "task-1", 0, 1)
	details := ErrorDetails(err)
	if details.Code != "INSUFFICIENT_BALANCE" || details.Message != "余额不足" {
		t.Fatalf("details = %#v", details)
	}
}

func TestMediaTransportLifecycleWithMockGateway(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"image-1","model_id":"model-1","model_type":"image"}]}`))
		case "/api/v1/files":
			_, _ = w.Write([]byte(`{"code":0,"data":{"download_url":"` + serverURL + `/input.png"}}`))
		case "/v1/images/generations":
			_, _ = w.Write([]byte(`{"data":{"task_id":"task-1"}}`))
		case "/v1/tasks/task-1":
			_, _ = w.Write([]byte(`{"data":{"status":"success","result":{"url":"` + serverURL + `/result.png"}}}`))
		case "/result.png":
			_, _ = w.Write([]byte("generated"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	c := NewWithEndpoints("token", server.URL, server.URL)
	if _, err := c.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(input, []byte("input"), 0600); err != nil {
		t.Fatal(err)
	}
	url, err := c.UploadFile(context.Background(), "input.png", input)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := c.SubmitTask(context.Background(), c.MediaEndpoint("/v1/images/generations"), "model-1", map[string]any{"images": []string{url}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := c.PollTask(context.Background(), taskID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	urls, err := ExtractResultURLs(body)
	if err != nil || len(urls) != 1 {
		t.Fatalf("urls=%v err=%v", urls, err)
	}
	output := filepath.Join(t.TempDir(), "result.png")
	if err := c.Download(context.Background(), urls[0], output); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != "generated" {
		t.Fatalf("output=%q err=%v", got, err)
	}
}
