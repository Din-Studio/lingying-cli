#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
skill="$root/skills/SKILL.md"

[[ -f "$skill" ]] || { echo "missing skills/SKILL.md" >&2; exit 1; }
grep -Fq 'name: lingying-gateway' "$skill"
grep -Fq 'ly --json task get' "$skill"
grep -Fq 'LY_ACCESS_TOKEN' "$skill"
# SKILL.md 必须随发布归档一同分发。分发渠道已从 npm 换成 GitHub Release，
# 因此契约的检查对象也从 package.json 的 files 换成 goreleaser 的 archives。
grep -Fq 'src: skills/SKILL.md' "$root/.goreleaser.yml"
grep -Fq 'dst: skills/SKILL.md' "$root/.goreleaser.yml"
echo "skill package contract passed"
