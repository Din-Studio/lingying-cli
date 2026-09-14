package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestChatSendsOptionalSchemaParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["temperature"] != float64(0.7) {
			t.Fatalf("temperature = %#v, want 0.7", payload["temperature"])
		}
		if payload["max_tokens"] != float64(2048) {
			t.Fatalf("max_tokens = %#v, want 2048", payload["max_tokens"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	reply, err := NewWithBaseURL("secret", server.URL).Chat(
		context.Background(), "model-1", "openai", []ChatMessage{{Role: "user", Content: "hello"}},
		map[string]any{"max_tokens": 2048, "temperature": 0.7},
	)
	if err != nil || reply != "ok" {
		t.Fatalf("Chat() = %q, %v", reply, err)
	}
}

func TestChatRejectsInvalidUTF8BeforeSendingRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	invalid := string([]byte{0xff})
	_, err := NewWithBaseURL("secret", server.URL).Chat(
		context.Background(), "model-1", "openai", []ChatMessage{{Role: "user", Content: invalid}}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("Chat() error = %v, want UTF-8 validation error", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, invalid text must not be sent", requests)
	}
}

func TestUploadFilePresignedSmallFlow(t *testing.T) {
	var mu sync.Mutex
	var putBody []byte
	var putContentType, putAuth string
	var completed bool
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/files/presigned":
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Errorf("init Authorization = %q", got)
			}
			var req struct {
				Name        string `json:"name"`
				Size        int64  `json:"size"`
				ContentType string `json:"content_type"`
				Hash        string `json:"hash"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode init request: %v", err)
			}
			// sha256("hello") 固定值，校验去重哈希按内容计算
			if req.Name != "hello.txt" || req.Size != 5 ||
				req.Hash != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
				t.Errorf("init request = %+v", req)
			}
			_, _ = w.Write([]byte(`{"file_id":"f-1","upload_url":"` + serverURL + `/put/f-1","status":"pending","deduplicated":false,"required_headers":{"Content-Type":"text/plain; charset=utf-8"}}`))
		case "PUT /put/f-1":
			putAuth = r.Header.Get("Authorization")
			putContentType = r.Header.Get("Content-Type")
			putBody, _ = io.ReadAll(r.Body)
		case "POST /v1/files/f-1/completion":
			completed = true
			_, _ = w.Write([]byte(`{"file_id":"f-1","status":"completed"}`))
		case "GET /v1/files/f-1/link":
			if got := r.URL.Query().Get("url_format"); got != "direct" {
				t.Errorf("link url_format = %q", got)
			}
			_, _ = w.Write([]byte(`{"download_url":"https://files.test/hello"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	url, err := NewWithBaseURL("secret", server.URL).UploadFile(context.Background(), "hello.txt", path)
	if err != nil || url != "https://files.test/hello" {
		t.Fatalf("UploadFile() = %q, %v", url, err)
	}
	if !bytes.Equal(putBody, []byte("hello")) {
		t.Fatalf("PUT body = %q", putBody)
	}
	if putAuth != "" {
		t.Fatalf("presigned PUT must not carry auth header, got %q", putAuth)
	}
	if putContentType != "text/plain; charset=utf-8" {
		t.Fatalf("PUT Content-Type = %q", putContentType)
	}
	if !completed {
		t.Fatal("completion was not called")
	}
}

func TestUploadFilePresignedMultipart(t *testing.T) {
	oldThreshold, oldPart, oldConc := multipartThreshold, uploadPartSize, uploadConcurrency
	multipartThreshold, uploadPartSize, uploadConcurrency = 8, 4, 2
	defer func() {
		multipartThreshold, uploadPartSize, uploadConcurrency = oldThreshold, oldPart, oldConc
	}()

	var mu sync.Mutex
	putBodies := map[string][]byte{} // part number → bytes
	var completedParts []struct {
		PartNumber int    `json:"part_number"`
		ETag       string `json:"etag"`
	}
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/files/multipart":
			var req struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Name != "big.bin" || req.Size != 10 {
				t.Errorf("multipart init = %+v", req)
			}
			_, _ = w.Write([]byte(`{"file_id":"f-3"}`))
		case r.Method == "POST" && r.URL.Path == "/v1/files/f-3/multipart/parts":
			var req struct {
				PartNumber int `json:"part_number"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"part_number":%d,"upload_url":"%s/put/part/%d"}`, req.PartNumber, serverURL, req.PartNumber)))
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/put/part/"):
			n := strings.TrimPrefix(r.URL.Path, "/put/part/")
			body, _ := io.ReadAll(r.Body)
			putBodies[n] = body
			w.Header().Set("ETag", `"etag-`+n+`"`)
		case r.Method == "POST" && r.URL.Path == "/v1/files/f-3/multipart/completion":
			var req struct {
				Parts []struct {
					PartNumber int    `json:"part_number"`
					ETag       string `json:"etag"`
				} `json:"parts"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			completedParts = req.Parts
			_, _ = w.Write([]byte(`{"status":"completed"}`))
		case r.Method == "GET" && r.URL.Path == "/v1/files/f-3/link":
			_, _ = w.Write([]byte(`{"download_url":"https://files.test/big"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	path := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil { // 10 字节 → 3 片：4+4+2
		t.Fatal(err)
	}
	url, err := NewWithBaseURL("secret", server.URL).UploadFile(context.Background(), "big.bin", path)
	if err != nil || url != "https://files.test/big" {
		t.Fatalf("UploadFile() = %q, %v", url, err)
	}
	if string(putBodies["1"]) != "0123" || string(putBodies["2"]) != "4567" || string(putBodies["3"]) != "89" {
		t.Fatalf("part bodies = %#v", putBodies)
	}
	if len(completedParts) != 3 {
		t.Fatalf("completed parts = %#v", completedParts)
	}
	for i, p := range completedParts {
		want := fmt.Sprintf(`"etag-%d"`, i+1)
		if p.PartNumber != i+1 || p.ETag != want {
			t.Fatalf("part %d = %+v, want etag %s", i, p, want)
		}
	}
}

func TestUploadFilePresignedDedupSkipsPut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/files/presigned":
			_, _ = w.Write([]byte(`{"file_id":"f-2","status":"completed","deduplicated":true}`))
		case "GET /v1/files/f-2/link":
			_, _ = w.Write([]byte(`{"download_url":"https://files.test/dedup"}`))
		default:
			t.Errorf("dedup path must not call %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	url, err := NewWithBaseURL("secret", server.URL).UploadFile(context.Background(), "hello.txt", path)
	if err != nil || url != "https://files.test/dedup" {
		t.Fatalf("UploadFile() = %q, %v", url, err)
	}
}

func TestUploadFilePresignedPutFailure(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/files/presigned":
			_, _ = w.Write([]byte(`{"file_id":"f-4","upload_url":"` + serverURL + `/put/f-4","status":"pending"}`))
		case "PUT /put/f-4":
			http.Error(w, "denied", http.StatusForbidden)
		default:
			t.Errorf("unexpected request after PUT failure: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewWithBaseURL("secret", server.URL).UploadFile(context.Background(), "hello.txt", path)
	if err == nil || !strings.Contains(err.Error(), "预签名 PUT HTTP 403") {
		t.Fatalf("err = %v, want 预签名 PUT HTTP 403", err)
	}
}

func TestUploadFileEmptyDownloadLinkFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/files/presigned":
			_, _ = w.Write([]byte(`{"file_id":"f-5","status":"completed","deduplicated":true}`))
		case "GET /v1/files/f-5/link":
			_, _ = w.Write([]byte(`{"download_url":""}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewWithBaseURL("secret", server.URL).UploadFile(context.Background(), "hello.txt", path)
	if err == nil || !strings.Contains(err.Error(), "下载链接为空") {
		t.Fatalf("err = %v, want 下载链接为空", err)
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
		case "/v1/files/presigned":
			_, _ = w.Write([]byte(`{"file_id":"f1","upload_url":"` + serverURL + `/put/f1","status":"pending"}`))
		case "/put/f1":
			// 预签名 PUT 目标，200 空响应即可
		case "/v1/files/f1/completion":
			_, _ = w.Write([]byte(`{"status":"completed"}`))
		case "/v1/files/f1/link":
			_, _ = w.Write([]byte(`{"download_url":"` + serverURL + `/input.png"}`))
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

	c := NewWithBaseURL("token", server.URL)
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

func TestUploadFileSurfacesGatewayErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"账户已被禁用，请联系管理员","error_code":"USER_DISABLED","data":{"error":"账户已被禁用，请联系管理员"}}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewWithBaseURL("secret", server.URL).UploadFile(context.Background(), "hello.txt", path)
	if err == nil {
		t.Fatal("expected an error")
	}
	detail := ErrorDetails(err)
	if detail.Code != "USER_DISABLED" {
		t.Fatalf("error_code must survive as a structured detail, got %q (err=%v)", detail.Code, err)
	}
}

func TestUploadSmallReplaysRequiredHeaders(t *testing.T) {
	var putHeaders http.Header
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/files/presigned":
			_, _ = w.Write([]byte(`{"file_id":"f1","upload_url":"` + srv.URL + `/put","required_headers":{"Content-Type":"application/octet-stream","Cache-Control":"public, max-age=31536000, immutable"}}`))
		case r.URL.Path == "/put":
			putHeaders = r.Header.Clone()
			w.Header().Set("ETag", `"e1"`)
		case strings.HasSuffix(r.URL.Path, "/completion"):
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{"download_url":"https://cdn/x"}`))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithBaseURL("secret", srv.URL).UploadFile(context.Background(), "a.bin", path); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// The server signs the Content-Type it derived itself, so the client must
	// send that value back rather than its own guess.
	if got := putHeaders.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type: got %q, want the server's signed value", got)
	}
	if got := putHeaders.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control: got %q, want it replayed", got)
	}
	if putHeaders.Get("Authorization") != "" {
		t.Fatal("presigned PUT must not carry an Authorization header")
	}
}

func TestUploadMultipartSendsNoContentTypeOnPartPUT(t *testing.T) {
	oldThreshold, oldPart := multipartThreshold, uploadPartSize
	multipartThreshold, uploadPartSize = 4, 4
	defer func() { multipartThreshold, uploadPartSize = oldThreshold, oldPart }()

	var partHeaders http.Header
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/files/multipart":
			_, _ = w.Write([]byte(`{"file_id":"f1"}`))
		case strings.HasSuffix(r.URL.Path, "/multipart/parts"):
			_, _ = w.Write([]byte(`{"upload_url":"` + srv.URL + `/put"}`))
		case r.URL.Path == "/put":
			partHeaders = r.Header.Clone()
			w.Header().Set("ETag", `"e1"`)
		case strings.HasSuffix(r.URL.Path, "/multipart/completion"):
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{"download_url":"https://cdn/x"}`))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(path, []byte("12345678"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithBaseURL("secret", srv.URL).UploadFile(context.Background(), "big.bin", path); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// OSS signs an empty content-type for part URLs, so sending one is a 403.
	if got := partHeaders.Get("Content-Type"); got != "" {
		t.Fatalf("part PUT must send no Content-Type, got %q", got)
	}
}
