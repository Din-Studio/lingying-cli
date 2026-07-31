#!/usr/bin/env bash
# ly install script
#
#   curl -fsSL https://lingying.cn/cli | bash
#
# Installs the latest ly from GitHub Releases. macOS/Linux/WSL supported.
# Falls back to go install when no prebuilt binary exists.
set -euo pipefail

PROG="ly"
REPO="lingying/ly-cli"
MANIFEST_URL="https://lingying.cn/cli/version.json"
DEFAULT_INSTALL_DIR="${LY_INSTALL_DIR:-$HOME/.local/bin}"
SKILL_DIR="${HOME}/.ly"

say()  { printf 'ly-installer: %s\n' "$*"; }
fail() { printf 'ly-installer: %s\n' "$*" >&2; exit 1; }
has()  { command -v "$1" >/dev/null 2>&1; }

# ── Platform ──
detect_platform() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin) case "$arch" in
      x86_64) echo "darwin-amd64" ;;
      arm64)  echo "darwin-arm64" ;;
      *)      fail "不支持的架构: $arch"
    esac ;;
    Linux) case "$arch" in
      x86_64)      echo "linux-amd64" ;;
      aarch64|arm64) echo "linux-arm64" ;;
      *) fail "不支持的架构: $arch"
    esac ;;
    *) fail "不支持的操作系统: $os (WSL/Git Bash 请用 npm 安装)" ;;
  esac
}

# ── Download ──
download() {
  local url="$1" out="$2"
  if has curl; then
    curl -fsSL --connect-timeout 10 --max-time 120 "$url" -o "$out"
  elif has wget; then
    wget -q --timeout=10 --tries=3 "$url" -O "$out"
  else
    fail "需要 curl 或 wget"
  fi
}

# ── Version from manifest ──
latest_info() {
  download "$MANIFEST_URL" /dev/stdout 2>/dev/null || true
}

# ── PATH ──
ensure_path() {
  local dir="$1"
  case ":$PATH:" in *":$dir:"*) return ;; esac
  local rc=""
  case "$(basename "${SHELL:-}")" in
    zsh)  rc="${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash) [ -f "$HOME/.bashrc" ] && rc="$HOME/.bashrc" || rc="$HOME/.bash_profile" ;;
    *)    rc="$HOME/.profile" ;;
  esac
  mkdir -p "$(dirname "$rc")"
  if ! grep -Fqx "export PATH=\"$dir:\$PATH\"" "$rc" 2>/dev/null; then
    printf '\n# ly\nexport PATH="%s:$PATH"\n' "$dir" >> "$rc"
    say "已将 $dir 写入 $rc"
    say "请执行: source $rc"
  fi
}

unquarantine() {
  [ "$(uname -s)" = "Darwin" ] && has xattr && xattr -d com.apple.quarantine "$1" 2>/dev/null || true
}

# ── Already installed? ──
if has ly; then
  current="$(ly --version 2>/dev/null | awk '{print $NF}' || echo "?")"
  say "ly $current 已安装。重新运行将覆盖更新。"
fi

platform="$(detect_platform)"

# ── Try manifest → get latest version + binary URL ──
version=""
bin_url=""
manifest="$(latest_info || true)"
if [ -n "$manifest" ]; then
  version="$(echo "$manifest" | grep -o '"version": *"[^"]*"' | head -1 | sed 's/.*"\(.*\)".*/\1/' || true)"
fi

if [ -n "${LY_VERSION:-}" ]; then
  version="$LY_VERSION"  # pin version via env var
fi

if [ -n "$version" ]; then
  archive="ly-${version#v}-${platform}.tar.gz"
  bin_url="https://github.com/${REPO}/releases/download/v${version#v}/${archive}"
fi

if [ -n "$bin_url" ]; then
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  say "平台: $platform  版本: ${version:-latest}"
  say "下载 $bin_url"

  if ! download "$bin_url" "$tmpdir/$archive" 2>/dev/null; then
    say "预编译包不存在，改用 go install..."
    bin_url=""
  else
    mkdir -p "$DEFAULT_INSTALL_DIR"
    tar -xzf "$tmpdir/$archive" -C "$tmpdir"
    find "$tmpdir" -type f -name "$PROG" -perm +111 -exec cp {} "$DEFAULT_INSTALL_DIR/$PROG" \; 2>/dev/null || \
    find "$tmpdir" -type f -perm +111 -exec cp {} "$DEFAULT_INSTALL_DIR/$PROG" \;
    chmod +x "$DEFAULT_INSTALL_DIR/$PROG"
    unquarantine "$DEFAULT_INSTALL_DIR/$PROG"
  fi
fi

# ── Fallback: go install ──
if [ -z "$bin_url" ]; then
  if has go; then
    say "go install ${REPO}@latest"
    GOBIN="$DEFAULT_INSTALL_DIR" go install "${REPO}@latest" 2>/dev/null || fail "go install 失败"
  else
    say "没有预编译包，也没有 Go 环境。"
    say "请用 npm 安装: npm install -g @lingying/cli"
    say "或安装 Go 后重试: go install ${REPO}@latest"
    exit 1
  fi
fi

ensure_path "$DEFAULT_INSTALL_DIR"

say "✅ ly 安装完成: $DEFAULT_INSTALL_DIR/$PROG"
say "   执行 ly --help 开始使用"

# ── Copy SKILL.md for Agent integration ──
skill_src="$(dirname "$0")/../skills/SKILL.md"
if [ -f "$skill_src" ]; then
  mkdir -p "$SKILL_DIR"
  cp "$skill_src" "$SKILL_DIR/SKILL.md"
  say "📋 SKILL.md 已复制到 $SKILL_DIR/SKILL.md (供 Agent 使用)"
fi
