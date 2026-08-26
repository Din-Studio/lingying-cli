// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Self-update: resolve the latest release, download it, replace this binary.
//
// The update path is deliberately identical for every install method. Whether
// ly arrived via install.sh or go install, we download the GitHub Release
// archive for this platform and overwrite the running executable in place.
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRepo = "Din-Studio/lingying-cli"

	// devVersion marks a binary built without the release ldflags. It carries
	// no comparable version, so any published release counts as newer.
	devVersion = "dev"

	// 版本发现只读一个 302 响应头，给它短超时。归档下载则不设总时长上限——
	// 那会在慢链路上把大文件硬砍断；改由 download.go 的停滞检测负责。
	versionTimeout = 10 * time.Second
	verifyTimeout  = 10 * time.Second
)

// Updater holds everything the update flow needs. Repo, Sources and the
// platform fields are configurable so tests can point at a local server.
type Updater struct {
	Current  string
	ExecPath string
	HTTP     *http.Client
	Repo     string
	Sources  []Source
	GOOS     string
	GOARCH   string
}

// New resolves the running executable — following symlinks so that a
// ~/.local/bin/ly symlink updates the real file, not the link.
func New(current string) (*Updater, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法定位当前可执行文件: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return &Updater{
		Current:  current,
		ExecPath: exe,
		// No client-level timeout: each request carries its own deadline.
		HTTP:    &http.Client{},
		Repo:    defaultRepo,
		Sources: Sources(),
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}, nil
}

// LatestVersion 读取 releases/latest 的 302 跳转目标。该地址的 Location 形如
// .../releases/tag/v0.1.6，一次请求即可拿到版本号——既不需要
// raw.githubusercontent.com 上的清单，也不需要 api.github.com。
func (u *Updater) LatestVersion(ctx context.Context) (string, error) {
	var lastErr error
	for _, src := range u.Sources {
		version, err := u.latestFrom(ctx, src)
		if err == nil {
			return version, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("无法获取最新版本: %w", lastErr)
}

func (u *Updater) latestFrom(ctx context.Context, src Source) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.LatestTagURL(u.Repo), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ly-cli-updater")

	// 只在这里禁用重定向跟随——我们要读的就是 Location 头。用副本而非改动
	// u.HTTP，否则归档下载也会被打断：GitHub 的 releases/download 本身就要
	// 跳转到 objects.githubusercontent.com。
	client := *u.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("%s 未返回跳转地址（HTTP %d）", src.Name, resp.StatusCode)
	}
	_, tag, found := strings.Cut(location, "/releases/tag/")
	if !found {
		return "", fmt.Errorf("%s 的跳转地址无法解析: %q", src.Name, location)
	}
	return validVersion(tag)
}

// NeedsUpdate reports whether latest is worth installing over the running
// build. A dev build has no comparable version, so it always updates.
func (u *Updater) NeedsUpdate(latest string) bool {
	current, err := validVersion(u.Current)
	if err != nil {
		return true
	}
	return compareVersions(latest, current) > 0
}

// Applied 描述一次成功更新的来源信息。
type Applied struct {
	SourceName string
	// ChecksumTrusted 为 false 表示校验和取自镜像，只能防传输损坏、不能防篡改。
	// 调用方必须就此向用户告警。
	ChecksumTrusted bool
}

// Apply downloads the release archive for `version`, verifies its SHA-256
// against checksums.txt, and replaces the running executable with it.
func (u *Updater) Apply(ctx context.Context, version string) (Applied, error) {
	version = strings.TrimPrefix(version, "v")
	dir := filepath.Dir(u.ExecPath)
	var applied Applied

	// Creating the staging file doubles as the writability probe: if we can't
	// write next to the executable, we can't atomically replace it either.
	staged, err := os.CreateTemp(dir, ".ly-update-*")
	if err != nil {
		return applied, fmt.Errorf("无法写入 %s: %w", dir, err)
	}
	stagedPath := staged.Name()
	staged.Close()
	defer os.Remove(stagedPath)

	asset := u.assetName()
	d := newDownloader()

	expected, trusted, err := u.fetchChecksum(ctx, d, version, asset)
	if err != nil {
		return applied, err
	}
	applied.ChecksumTrusted = trusted

	archivePath := stagedPath + ".archive"
	defer os.Remove(archivePath)

	var lastErr error
	for _, src := range u.Sources {
		_ = os.Remove(archivePath)
		if err := d.fetch(ctx, src.AssetURL(u.Repo, version, asset), archivePath); err != nil {
			lastErr = err
			continue
		}
		applied.SourceName = src.Name
		lastErr = nil
		break
	}
	if applied.SourceName == "" {
		return applied, fmt.Errorf("下载 %s 失败: %w", asset, lastErr)
	}

	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return applied, err
	}
	actual := sha256.Sum256(archive)
	if hex.EncodeToString(actual[:]) != expected {
		// 校验失败是安全事件，不换源重试——立刻中止。
		return applied, fmt.Errorf("校验和不匹配，已中止更新\n  期望: %s\n  实际: %s", expected, hex.EncodeToString(actual[:]))
	}

	if err := extractBinary(archive, asset, u.binaryName(), stagedPath); err != nil {
		return applied, err
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return applied, err
	}
	if err := verifyStaged(ctx, stagedPath, version); err != nil {
		return applied, err
	}
	return applied, replaceExecutable(stagedPath, u.ExecPath, u.GOOS == "windows")
}

// fetchChecksum 先只向可信源索取 checksums.txt。校验和文件很小，慢链路上也容易
// 直连成功；只要它来自可信源，归档包就可以安全地走镜像——镜像换不掉包。
//
// 降级到镜像只在可信源「不可用」时发生，且会把 trusted 置为 false 供调用方告警。
// 「取不到 checksums.txt」与「取到了但其中没有该资产」必须区别对待：后者是一个
// 确定性答复——该资产不存在。此时若改用镜像的 checksums.txt，镜像便能自造资产名
// 与配套归档，而用户只看到一行降级警告。因此可信源一旦给出确定性答复即失败。
func (u *Updater) fetchChecksum(ctx context.Context, d *downloader, version, asset string) (string, bool, error) {
	try := func(want bool) (sum string, ok bool, definitive error) {
		for _, src := range u.Sources {
			if src.Trusted != want {
				continue
			}
			sums, err := d.bytes(ctx, src.AssetURL(u.Repo, version, "checksums.txt"))
			if err != nil {
				continue // 来源不可用，换下一个
			}
			// 代理挂掉时常返回 200 加一张 HTML 错误页。那不是确定性答复，
			// 只说明这个来源不可用；否则一个坏代理就能掐断整条回退链。
			if !looksLikeChecksums(sums) {
				continue
			}
			sum, err := expectedChecksum(sums, asset)
			if err != nil {
				return "", false, err // 确定性答复：资产不存在
			}
			return sum, true, nil
		}
		return "", false, nil
	}

	sum, ok, definitive := try(true)
	if definitive != nil {
		return "", false, definitive
	}
	if ok {
		return sum, true, nil
	}

	sum, ok, definitive = try(false)
	if definitive != nil {
		return "", false, definitive
	}
	if ok {
		return sum, false, nil
	}
	return "", false, errors.New("下载校验和失败：所有来源均不可用")
}

// verifyStaged runs the downloaded binary before it takes over. The checksum
// proves the download matches what was published; it cannot prove the
// published artifact runs on this machine. Without this check a bad release
// leaves the user with a broken ly and no working `ly update` to recover with.
func verifyStaged(ctx context.Context, path, version string) error {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("新版本无法运行，已保留当前版本: %w", err)
	}
	if !strings.Contains(string(out), version) {
		return fmt.Errorf("新版本自报版本号不符（期望 %s，实际 %q），已保留当前版本",
			version, strings.TrimSpace(string(out)))
	}
	return nil
}

// assetName 返回本平台的归档名。名称不含版本号——版本由 URL 路径承载，
// 使 latest/download/X 与 download/v<X>/X 指向同一份文件。
func (u *Updater) assetName() string {
	ext := ".tar.gz"
	if u.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("ly-%s-%s%s", u.GOOS, u.GOARCH, ext)
}

func (u *Updater) binaryName() string {
	if u.GOOS == "windows" {
		return "ly.exe"
	}
	return "ly"
}

// looksLikeChecksums 判断响应体是否确实是一份 checksums.txt——只要存在一行
// 「64 位十六进制 + 文件名」即可。用来把「这不是校验和文件」与「是校验和文件
// 但没有该资产」区分开，两者的正确处置相反。
func looksLikeChecksums(sums []byte) bool {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(fields[0]); err == nil {
			return true
		}
	}
	return false
}

// expectedChecksum pulls the hash for `asset` out of goreleaser's
// checksums.txt, whose lines are "<sha256>  <filename>".
func expectedChecksum(sums []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != asset {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != 64 {
			return "", fmt.Errorf("checksums.txt 中 %s 的校验和格式无效: %q", asset, fields[0])
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return "", fmt.Errorf("checksums.txt 中 %s 的校验和格式无效: %q", asset, fields[0])
		}
		return sum, nil
	}
	return "", fmt.Errorf("checksums.txt 中未找到 %s", asset)
}

// extractBinary writes the single archive entry named `binary` to dest.
// Archive paths are only ever compared, never used as an output path, so a
// crafted entry name cannot escape the destination.
func extractBinary(archive []byte, asset, binary, dest string) error {
	if strings.HasSuffix(asset, ".zip") {
		return extractFromZip(archive, binary, dest)
	}
	return extractFromTarGz(archive, binary, dest)
}

func extractFromTarGz(archive []byte, binary, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("解压归档失败: %w", err)
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("读取归档失败: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binary {
			continue
		}
		return writeFile(dest, reader)
	}
	return fmt.Errorf("归档中未找到 %s", binary)
}

func extractFromZip(archive []byte, binary, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("解压归档失败: %w", err)
	}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || filepath.Base(entry.Name) != binary {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			return fmt.Errorf("读取归档失败: %w", err)
		}
		defer src.Close()
		return writeFile(dest, src)
	}
	return fmt.Errorf("归档中未找到 %s", binary)
}

func writeFile(dest string, src io.Reader) error {
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

// replaceExecutable swaps staged into place. Windows refuses to overwrite a
// running executable but does allow renaming it, so the old binary is moved
// aside first and cleaned up on a later run.
func replaceExecutable(staged, target string, windows bool) error {
	if !windows {
		return os.Rename(staged, target)
	}
	backup := target + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

// validVersion accepts dotted numeric versions with an optional "v" prefix and
// an optional pre-release suffix ("v1.2.3", "1.2.3-rc1"), returning the
// normalized "1.2.3" form.
func validVersion(raw string) (string, error) {
	version := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if base, _, found := strings.Cut(version, "-"); found {
		version = base
	}
	if version == "" || version == devVersion {
		return "", fmt.Errorf("版本号无效: %q", raw)
	}
	for _, segment := range strings.Split(version, ".") {
		if _, err := strconv.Atoi(segment); err != nil {
			return "", fmt.Errorf("版本号无效: %q", raw)
		}
	}
	return version, nil
}

// compareVersions returns >0 when a is newer than b, 0 when equal, <0 when
// older. Inputs are assumed to have passed validVersion.
func compareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if diff := segment(as, i) - segment(bs, i); diff != 0 {
			return diff
		}
	}
	return 0
}

func segment(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}
