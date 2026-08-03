# Agent-Safe CLI Design

## Goal

Make `ly` a conventional, scriptable Gateway CLI whose `--json` interface can
be used reliably by Agents without the browser HTTP bridge assumed by the
reference Skill.

## Command contract

`ly --json` writes exactly one JSON envelope to stdout. Diagnostics and
progress go to stderr. Every outcome uses `ok`, `data`, and optional `meta`.
Failures expose a stable `code` and safe `message`; an accepted asynchronous
task also exposes `task_id` so callers can resume it.

Credentials continue to resolve from `LY_ACCESS_TOKEN`, `LY_API_KEY`, then the
local configuration file. Documentation makes environment variables the
recommended non-interactive Agent authentication mechanism.

## Model and request selection

Each text or media invocation discovers models from `/v1/models`. A requested
model must match by canonical ID or exact, case-insensitive display name and
must have an eligible model type. A missing or mismatched model is an error;
the CLI must never select a fallback model. Image and video commands accept
both their generation and editing model types. Audio accepts `audio` and
`audio_edit`.

`--dry-run` performs discovery and request construction only. It must not
upload files or submit tasks. Local input files require OAuth; URL inputs work
with either credential type.

## Media lifecycle

Media commands submit an eligible schema-shaped request, poll the returned
task ID, extract output URLs from the task result, and download outputs to
unique local filenames unless `--output` is provided. On timeout or caller
interruption they return a JSON error that retains the task ID.

`ly task get <task_id>` queries an existing task. It reports status and result
in JSON and, when an output path is requested and result URLs are present,
downloads them. This creates a recoverable boundary between task submission
and collection.

## Scope limits

This change removes the misleading `ly text --file` option rather than
inventing a text-file transport not supported by the current Gateway client.
Schema validation remains server-authoritative; the CLI rejects only malformed
CLI arguments and impossible local-file/auth combinations.

## Verification

Add HTTP-server-backed Go tests covering JSON-only error output, strict model
selection, dry-run side-effect prevention, task URL extraction/download, and
task status retrieval. Build and run the full Go test suite after the local Go
toolchain is made consistent.
