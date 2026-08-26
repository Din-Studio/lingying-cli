#!/usr/bin/env bash
# ly 引导安装器
#
#   curl -fsSL https://github.com/Din-Studio/lingying-cli/releases/latest/download/install.sh | bash
#
# 唯一职责：取得一个校验通过的 ly 并写入 PATH。此后的更新请用 ly update。
# 环境变量：LY_VERSION 锁版本、LY_MIRROR 自定义镜像、LY_INSTALL_DIR 安装目录。
set -euo pipefail

REPO="Din-Studio/lingying-cli"
DIRECT="https://github.com"
INSTALL_DIR="${LY_INSTALL_DIR:-$HOME/.local/bin}"

say()  { printf 'ly-installer: %s\n' "$*"; }
fail() { printf 'ly-installer: %s\n' "$*" >&2; exit 1; }

platform() {
  case "$(uname -s)/$(uname -m)" in
    Darwin/x86_64)             echo "darwin-amd64" ;;
    Darwin/arm64)              echo "darwin-arm64" ;;
    Linux/x86_64)              echo "linux-amd64" ;;
    Linux/aarch64|Linux/arm64) echo "linux-arm64" ;;
    *) fail "不支持的平台: $(uname -s)/$(uname -m)" ;;
  esac
}

# 候选源基址，直连优先。顺序必须与 internal/updater/source.go 的 Sources() 一致。
sources() {
  echo "$DIRECT"
  [ -n "${LY_MIRROR:-}" ] && echo "${LY_MIRROR%/}/$DIRECT"
  echo "https://ghfast.top/$DIRECT"
  echo "https://gh-proxy.com/$DIRECT"
}

# 资产名不含版本号：latest/download/X 只是到 download/<最新tag>/X 的重定向，
# 因此无需先发现版本号即可下载。
asset_url() {
  local base="$1" asset="$2"
  if [ -n "${LY_VERSION:-}" ]; then
    echo "${base}/${REPO}/releases/download/v${LY_VERSION#v}/${asset}"
  else
    echo "${base}/${REPO}/releases/latest/download/${asset}"
  fi
}

# --speed-limit/--speed-time 只在真正停滞时放弃，取代会把大文件硬砍断的
# --max-time；-C - 让重试从断点续传而不是从头再来。
get() {
  curl -fsSL --connect-timeout 15 --retry 3 --retry-delay 1 --retry-all-errors \
       --speed-limit 1024 --speed-time 30 -C - "$1" -o "$2"
}

sha256_of() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else fail "需要 shasum 或 sha256sum"; fi
}

command -v curl >/dev/null 2>&1 || fail "需要 curl"

PLATFORM="$(platform)"
ASSET="ly-${PLATFORM}.tar.gz"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# checksums.txt 只有几百字节，慢链路上也容易直连成功。先只向直连索取：
# 校验和一旦来自可信源，归档包就可以安全地走镜像——镜像换不掉包。
# 每次换源前必须清掉残片：get 带 -C - 断点续传，若上一个来源传了一半就断，
# 换源后会把新来源的后半段追加到旧残片上，拼出一个损坏的校验和文件。
TRUSTED=1
rm -f "${TMP:?}/checksums.txt"
if ! get "$(asset_url "$DIRECT" checksums.txt)" "$TMP/checksums.txt" 2>/dev/null; then
  TRUSTED=0
  while IFS= read -r base; do
    [ "$base" = "$DIRECT" ] && continue
    rm -f "${TMP:?}/checksums.txt"
    get "$(asset_url "$base" checksums.txt)" "$TMP/checksums.txt" 2>/dev/null && break
  done < <(sources)
fi
[ -s "$TMP/checksums.txt" ] || fail "无法获取校验和文件，已中止以免安装未经校验的二进制"

EXPECTED="$(awk -v a="$ASSET" '{sub(/^\*/,"",$2); if ($2==a) print tolower($1)}' "$TMP/checksums.txt")"
echo "$EXPECTED" | grep -qEx '[0-9a-f]{64}' || fail "校验和文件中没有 $ASSET"

say "平台 $PLATFORM  版本 ${LY_VERSION:-latest}"
DOWNLOADED=0
while IFS= read -r base; do
  say "下载 ${ASSET}（${base}）"
  rm -f "${TMP:?}/$ASSET"
  if get "$(asset_url "$base" "$ASSET")" "$TMP/$ASSET" 2>/dev/null; then DOWNLOADED=1; break; fi
  say "该来源不可用，尝试下一个"
done < <(sources)
[ "$DOWNLOADED" = 1 ] || fail "所有下载来源均失败"

[ "$(sha256_of "$TMP/$ASSET")" = "$EXPECTED" ] || fail "SHA-256 校验和不匹配，已中止"
say "SHA-256 校验通过"
[ "$TRUSTED" = 1 ] || say "警告 —— 校验和取自镜像而非 GitHub 直连，只能防传输损坏，不能防篡改"

tar -xzf "$TMP/$ASSET" -C "$TMP"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP/ly" "$INSTALL_DIR/ly"
if [ "$(uname -s)" = "Darwin" ]; then
  xattr -d com.apple.quarantine "$INSTALL_DIR/ly" 2>/dev/null || true
fi

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    case "$(basename "${SHELL:-sh}")" in
      zsh)  RC="${ZDOTDIR:-$HOME}/.zshrc" ;;
      bash) if [ -f "$HOME/.bashrc" ]; then RC="$HOME/.bashrc"; else RC="$HOME/.bash_profile"; fi ;;
      *)    RC="$HOME/.profile" ;;
    esac
    # shellcheck disable=SC2016  # $PATH 要以字面量写进 rc 文件，不能在此展开
    printf '\n# ly\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >> "$RC"
    say "已将 $INSTALL_DIR 写入 ${RC}，请执行: source $RC"
    ;;
esac

say "ly 已安装: $INSTALL_DIR/ly"
say "后续更新请运行: ly update"
