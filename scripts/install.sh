#!/usr/bin/env bash
# ly install script
#
#   curl -fsSL https://raw.githubusercontent.com/Din-Studio/lingying-cli/main/scripts/install.sh | bash
#
# Installs the latest ly from GitHub Releases. macOS/Linux/WSL supported.
# Falls back to go install when no prebuilt binary exists.
set -euo pipefail

PROG="ly"
REPO="Din-Studio/lingying-cli"
RELEASE_BASE="https://github.com/${REPO}/releases/download"
MANIFEST_URL="https://raw.githubusercontent.com/${REPO}/main/scripts/version.json"
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

# ── Download (no redirect following — curl -L disabled) ──
download() {
  local url="$1" out="$2"
  if has curl; then
    # -fsSL: fail on error, silent, follow redirects, but we validate below.
    # We use --max-redirs to limit and capture the effective URL.
    curl -fsSL --connect-timeout 10 --max-time 120 --max-redirs 5 "$url" -o "$out"
  elif has wget; then
    wget -q --timeout=10 --tries=3 --max-redirect=5 "$url" -O "$out"
  else
    fail "需要 curl 或 wget"
  fi
}

# ── Verify SHA-256 checksum ──
# Args: file_path expected_hex_hash
verify_checksum() {
  local file="$1" expected="$2"
  local actual
  if has shasum; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  elif has sha256sum; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  else
    fail "需要 shasum 或 sha256sum 进行校验"
  fi
  if [ "$actual" != "$expected" ]; then
    fail "校验和不匹配!\n  期望: $expected\n  实际: $actual"
  fi
}

# ── Version from manifest (optional) ──
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

# ── Try manifest → get latest version ──
version=""
manifest="$(latest_info || true)"
if [ -n "$manifest" ]; then
  version="$(echo "$manifest" | grep -o '"version": *"[^"]*"' | head -1 | sed 's/.*"\(.*\)".*/\1/' || true)"
fi

if [ -n "${LY_VERSION:-}" ]; then
  version="$LY_VERSION"  # pin version via env var
fi

if [ -z "$version" ]; then
  # Fallback: try to discover the latest release tag from GitHub API.
  # This is a best-effort — if it fails we fall through to go install.
  if has curl; then
    version="$(curl -fsSL --connect-timeout 10 --max-time 15 \
      "https://api.github.com/repos/${REPO}/releases/latest" \
      2>/dev/null | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/' || true)"
  fi
fi

bin_url=""
checksum_url=""
archive=""
expected_hash=""

if [ -n "$version" ]; then
  archive="ly-${version#v}-${platform}.tar.gz"
  bin_url="${RELEASE_BASE}/v${version#v}/${archive}"
  checksum_url="${RELEASE_BASE}/v${version#v}/checksums.txt"
fi

if [ -n "$bin_url" ]; then
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  say "平台: $platform  版本: ${version:-latest}"

  # 1. Download checksums.txt.
  say "下载校验和 $checksum_url"
  if download "$checksum_url" "$tmpdir/checksums.txt" 2>/dev/null; then
    # Parse out the expected hash for our archive.
    expected_hash="$(grep -F "  ${archive}" "$tmpdir/checksums.txt" 2>/dev/null | awk '{print $1}' || true)"
    if [ -z "$expected_hash" ]; then
      fail "校验和文件中未找到 ${archive}"
    fi
    if ! echo "$expected_hash" | grep -qEx '[0-9a-fA-F]{64}'; then
      fail "校验和格式无效: $expected_hash"
    fi
  else
    fail "无法下载校验和文件，已取消安装以避免安装未经校验的二进制"
  fi

  # 2. Download binary archive.
  say "下载 $bin_url"
  if ! download "$bin_url" "$tmpdir/$archive" 2>/dev/null; then
    say "预编译包不存在，改用 go install..."
    bin_url=""
  else
    # 3. Verify checksum if we have one.
    if [ -n "$expected_hash" ]; then
      say "验证 SHA-256 校验和..."
      verify_checksum "$tmpdir/$archive" "$expected_hash"
      say "✅ 校验和验证通过"
    fi

    # 4. Extract with path traversal guard.
    mkdir -p "$DEFAULT_INSTALL_DIR"
    # Guard: check archive entries for path traversal before extracting.
    if tar -tzf "$tmpdir/$archive" 2>/dev/null | grep -qE '(^/|\.\.)' ; then
      fail "归档包含路径遍历攻击: $archive"
    fi
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
    say "请用 npm 安装: npm install -g lingying-cli"
    say "或安装 Go 后重试: go install ${REPO}@latest"
    exit 1
  fi
fi

ensure_path "$DEFAULT_INSTALL_DIR"

say "✅ ly 安装完成: $DEFAULT_INSTALL_DIR/$PROG"
say "   执行 ly --help 开始使用"
