#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
skill="$root/skills/SKILL.md"

[[ -f "$skill" ]] || { echo "missing skills/SKILL.md" >&2; exit 1; }
grep -Fq 'name: lingying-gateway' "$skill"
grep -Fq 'ly --json task get' "$skill"
grep -Fq 'LY_ACCESS_TOKEN' "$skill"
node -e 'const p=require(process.argv[1]); if (!p.files.includes("skills/SKILL.md")) process.exit(1)' "$root/package.json"
echo "skill package contract passed"
