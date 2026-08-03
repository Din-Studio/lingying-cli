# Agent-Safe CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `ly --json` into a strict, recoverable contract for Agent callers.

**Architecture:** Centralize CLI JSON success/failure emission and Gateway model selection. Media commands become explicit submit/poll/download flows, with `task get` providing recovery after a timeout. Tests use local HTTP servers through an injectable Gateway base URL.

**Tech Stack:** Go 1.23+, Cobra, standard-library `httptest`.

---

### Task 1: Establish testable Gateway and JSON primitives

**Files:**
- Modify: `internal/client/gateway.go`
- Modify: `internal/output/envelope.go`
- Create: `internal/client/gateway_test.go`

- [ ] Add a test that overrides the Gateway base URL and asserts `ListModels` targets the override and preserves Bearer authentication.
- [ ] Run `go test ./internal/client -run TestListModels -v` and confirm it fails before the override exists.
- [ ] Add a testable `BaseURL` client option and move hard-coded endpoint construction behind client methods.
- [ ] Add `output.Failure(code, message, data)` so JSON failures have stable `code` and `message` fields.
- [ ] Re-run the focused test and then `go test ./internal/client ./internal/output`.

### Task 2: Enforce strict discovery and safe JSON command behavior

**Files:**
- Modify: `cmd/root.go`
- Modify: `cmd/text.go`
- Modify: `cmd/model.go`
- Modify: `cmd/media.go`
- Create: `cmd/contract_test.go`

- [ ] Write failing tests for an unknown media model, a model of the wrong type, and a model-list transport failure under `--json`.
- [ ] Run `go test ./cmd -run 'Test.*JSON|Test.*Model' -v` and verify each test fails against the current fallback/raw-error behavior.
- [ ] Route command failures through one JSON-aware helper, emit progress only to stderr, and match models only by canonical ID or exact case-insensitive display name.
- [ ] Include `image_edit`, `video_edit`, `audio`, and `audio_edit` in their respective media candidate sets; remove the unused text `--file` flag.
- [ ] Re-run the focused command tests.

### Task 3: Make media submission recoverable and download results

**Files:**
- Modify: `internal/client/gateway.go`
- Modify: `cmd/media.go`
- Create: `cmd/media_test.go`

- [ ] Write failing tests that assert dry-run never calls upload/submit, a completed task downloads result URLs, and timeout JSON retains `task_id`.
- [ ] Run `go test ./cmd -run 'TestMediaDryRun|TestMediaDownloads|TestMediaTimeout' -v` and confirm failure.
- [ ] Add task result URL extraction, unique default output naming, URL downloads, and task ID preservation on polling errors.
- [ ] Move the dry-run return before any upload and direct human-readable progress to stderr.
- [ ] Re-run the focused media tests.

### Task 4: Add task-status recovery

**Files:**
- Create: `cmd/task.go`
- Modify: `cmd/root.go`
- Create: `cmd/task_test.go`

- [ ] Write a failing test for `ly --json task get <id>` that returns task status, result, and metadata from a mocked task endpoint.
- [ ] Run `go test ./cmd -run TestTaskGet -v` and verify the command is absent.
- [ ] Implement `task get <task_id>` using the shared client; return a JSON envelope for pending, success, and failure without re-submitting work.
- [ ] Re-run the focused task test.

### Task 5: Align the packaged Agent contract and release assets

**Files:**
- Modify: `README.md`
- Modify: `skills/SKILL.md`
- Modify: `package.json`
- Modify: `scripts/install.sh`
- Modify: `scripts/install.js`
- Create: `scripts/validate-skill.sh`

- [ ] Write a shell validation test asserting the distributed skill is in the npm file list and the documented commands exist in `./ly --help` when a build is available.
- [ ] Update docs to recommend environment credential variables, strict JSON handling, `task get`, recovery behavior, and real supported file input.
- [ ] Include `skills/SKILL.md` in npm packaging and remove the unsupported curl-installer copy behavior or package the Skill explicitly.
- [ ] Run `scripts/validate-skill.sh` and inspect `npm pack --dry-run`.

### Task 6: Full verification and completion checkpoint

**Files:**
- Verify: all changed files

- [ ] Run `gofmt -w` on changed Go files.
- [ ] Run `go test ./...`, `go vet ./...`, `make build`, focused CLI help checks, `scripts/validate-skill.sh`, and `npm pack --dry-run`.
- [ ] If the host Go cache remains version-inconsistent, report the exact failure and run all available non-build verifications without claiming a clean Go build.
- [ ] Commit the implementation with only task-related files staged.
