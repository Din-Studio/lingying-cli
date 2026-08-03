# Cross-Platform File I/O Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Provide bounded, streaming, recoverable local uploads and downloads on macOS, Linux, and Windows.

### Task 1: Tests

- [ ] Add `httptest` coverage for successful multipart upload, HTTP upload failure, size rejection, successful download replacement, and failed-download preservation.
- [ ] Run `GOTOOLCHAIN=go1.26.5 go test ./internal/client -run 'TestUpload|TestDownload' -v` and observe failures before implementation.

### Task 2: Streaming transfer implementation

- [ ] Add a 500 MiB limit, configured file-service endpoint for tests, explicit multipart/read/close/status checks, and streamed response writes in `internal/client/gateway.go`.
- [ ] Implement temporary-file cleanup and platform-aware destination replacement.
- [ ] Run the focused tests, then `GOTOOLCHAIN=go1.26.5 go test ./...`, `go vet ./...`, and `make build`.
