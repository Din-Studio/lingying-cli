package client

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestExtractResultURLsFindsNestedMediaURLs(t *testing.T) {
	urls, err := ExtractResultURLs([]byte(`{"data":{"result":{"files":[{"url":"https://example.test/a.png"}],"nested":{"download_url":"https://example.test/b.png"}}}}`))
	if err != nil {
		t.Fatalf("ExtractResultURLs() error = %v", err)
	}
	if len(urls) != 2 || urls[0] != "https://example.test/a.png" || urls[1] != "https://example.test/b.png" {
		t.Fatalf("urls = %#v", urls)
	}
}
