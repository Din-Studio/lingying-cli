# README Product Documentation Design

## Goal

Make the repository README a reliable product entry point for both terminal
users and agents. Every command, configuration rule, and automation contract
shown in the document must match the current CLI implementation.

## Information architecture

The README will be reorganized into these user-facing sections:

1. Product scope: dynamic model discovery through the Lingying Gateway; the
   CLI does not own capability catalogs or billing.
2. Installation: curl, npm, Go, and source builds, followed by `ly --help`.
   npm uses the current package name `lingying-cli`.
3. Quick start: authenticate, check connectivity, run text, then submit a
   media task.
4. Commands and media workflow: text/model discovery, local versus remote
   media input, the 1 GiB local upload limit, `--no-wait`, `task get`, and
   output-directory behavior.
5. Authentication and configuration: fixed platform paths, `--config`,
   `LY_CONFIG_FILE`, credential precedence, and `logout` semantics.
6. Agent integration: one JSON envelope on stdout, non-zero status for
   failures, stable error fields, Gateway error passthrough, and task recovery.
7. Troubleshooting and development verification.

## Reference-product comparison

Dreamina's published installer is a useful reference for platform detection,
PATH guidance, macOS quarantine handling, and installing a companion SKILL.
Lingying already covers the equivalent distribution concerns through checksum
verification, archive traversal checks, and installation fallbacks. The README
will document those existing capabilities without claiming unsupported
behaviour.

The Dreamina installer also mutates an OpenClaw workspace file. Lingying must
not copy this: an installer may place the CLI and its bundled skill, but it
must not modify agent instructions or a user's workspace without an explicit
command and consent.

## Accuracy rules

- Do not describe local schema validation, capability caches, OAuth device
  flow, or CLI billing; none are implemented or desired.
- Do not promise multipart streaming changes beyond the existing presigned
  upload path.
- Use the actual `lingying-cli` npm package name.
- Present Gateway errors such as `INSUFFICIENT_BALANCE` as upstream decisions
  that the CLI preserves, not errors the CLI interprets or charges for.

## Verification

Validate examples against `--help`, inspect package metadata, run Markdown
consistency checks, and run the project test/build suite after the document
change.
