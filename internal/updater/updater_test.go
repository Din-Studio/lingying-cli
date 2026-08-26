package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testUpdater points an Updater at a local server, with a throwaway file
// standing in for the running executable. GOOS/GOARCH stay at their real
// values so Apply exercises the same asset name and smoke test as production.
func testUpdater(t *testing.T, handler http.Handler) (*Updater, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	target := filepath.Join(dir, "ly")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Updater{
		Current:  "0.1.5",
		ExecPath: target,
		HTTP:     server.Client(),
		Repo:     "Din-Studio/lingying-cli",
		Sources:  []Source{{Name: "假直连", Base: server.URL, Trusted: true}},
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
	}, server.URL
}

// testAsset mirrors Updater.assetName for the platform the test runs on.
func testAsset() string {
	return fmt.Sprintf("ly-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
}

// fakeBinary is a runnable stand-in for a released ly: it answers --version
// the way cobra does, so the pre-swap smoke test sees a real process.
func fakeBinary(version string) string {
	return "#!/bin/sh\necho \"ly version " + version + "\"\n"
}

// requirePOSIXShell skips tests that rely on fakeBinary being executable.
func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fakeBinary is a POSIX shell script")
	}
}

// tarGz builds a release archive shaped like goreleaser's: the binary sits
// alongside LICENSE and a nested skills/ directory.
func tarGz(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	entries := []struct{ name, body string }{
		{"LICENSE", "MIT"},
		{"skills/SKILL.md", "# skill"},
		{name, content},
	}
	for _, entry := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name: entry.name, Mode: 0o755, Size: int64(len(entry.body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ── Version resolution ──

// redirectTo 模拟 GitHub 的 releases/latest：302 跳到具体 tag。
func redirectTo(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.invalid/x/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}
}

func serviceUnavailable() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func TestLatestVersionReadsTagFromRedirect(t *testing.T) {
	var sawPath string
	up, _ := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		redirectTo("v0.1.6")(w, r)
	}))

	got, err := up.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if got != "0.1.6" {
		t.Fatalf("LatestVersion() = %q, want %q", got, "0.1.6")
	}
	if !strings.HasSuffix(sawPath, "/releases/latest") {
		t.Fatalf("requested path = %q, want it to end with /releases/latest", sawPath)
	}
}

func TestLatestVersionFallsBackToNextSource(t *testing.T) {
	up, _ := testUpdater(t, serviceUnavailable())
	live := httptest.NewServer(redirectTo("v9.9.9"))
	defer live.Close()

	up.Sources = append(up.Sources, Source{Name: "可用镜像", Base: live.URL})

	got, err := up.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if got != "9.9.9" {
		t.Fatalf("LatestVersion() = %q, want %q — 直连挂掉时应回退到下一个来源", got, "9.9.9")
	}
}

func TestLatestVersionFailsWhenEverySourceIsDown(t *testing.T) {
	up, _ := testUpdater(t, serviceUnavailable())
	if _, err := up.LatestVersion(context.Background()); err == nil {
		t.Fatal("LatestVersion() = nil error, want failure")
	}
}

func TestNeedsUpdate(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.5", "0.1.6", true},
		{"0.1.5", "0.2.0", true},
		{"0.1.5", "1.0.0", true},
		{"0.1.5", "0.1.5", false},
		{"0.1.5", "0.1.4", false},
		{"0.2.0", "0.1.9", false},
		{"0.1.9", "0.1.10", true}, // numeric compare, not lexical
		{"0.1", "0.1.0", false},
		{"dev", "0.1.5", true}, // unversioned local build always updates
	}
	for _, tc := range cases {
		up := &Updater{Current: tc.current}
		if got := up.NeedsUpdate(tc.latest); got != tc.want {
			t.Errorf("current=%s latest=%s: NeedsUpdate() = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

// ── Checksums ──

func TestExpectedChecksum(t *testing.T) {
	valid := strings.Repeat("a", 64)
	sums := []byte(valid + "  ly-0.2.0-linux-amd64.tar.gz\n" +
		strings.Repeat("b", 64) + "  ly-0.2.0-darwin-arm64.tar.gz\n")

	got, err := expectedChecksum(sums, "ly-0.2.0-linux-amd64.tar.gz")
	if err != nil || got != valid {
		t.Fatalf("expectedChecksum() = %q, %v", got, err)
	}

	if _, err := expectedChecksum(sums, "ly-0.2.0-windows-amd64.zip"); err == nil {
		t.Fatal("missing asset: want error")
	}

	short := []byte("abc  ly-0.2.0-linux-amd64.tar.gz\n")
	if _, err := expectedChecksum(short, "ly-0.2.0-linux-amd64.tar.gz"); err == nil {
		t.Fatal("malformed hash: want error")
	}

	nonHex := []byte(strings.Repeat("z", 64) + "  ly-0.2.0-linux-amd64.tar.gz\n")
	if _, err := expectedChecksum(nonHex, "ly-0.2.0-linux-amd64.tar.gz"); err == nil {
		t.Fatal("non-hex hash: want error")
	}
}

// ── Apply ──

// serveRelease answers checksums.txt and the archive for `version`, and 404s
// everything else.
func serveRelease(version string, archive []byte) http.HandlerFunc {
	asset := testAsset()
	// 按后缀匹配：Source.AssetURL 拼出的路径带有仓库前缀
	// （/<owner>/<repo>/releases/download/v<版本>/<资产>）。
	prefix := "/releases/download/v" + version + "/"
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, prefix+"checksums.txt"):
			w.Write([]byte(sha256Hex(archive) + "  " + asset + "\n"))
		case strings.HasSuffix(r.URL.Path, prefix+asset):
			w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestApplyReplacesExecutable(t *testing.T) {
	requirePOSIXShell(t)
	binary := fakeBinary("0.2.0")
	archive := tarGz(t, "ly", binary)
	up, _ := testUpdater(t, serveRelease("0.2.0", archive))

	if _, err := up.Apply(context.Background(), "0.2.0"); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	got, err := os.ReadFile(up.ExecPath)
	if err != nil || string(got) != binary {
		t.Fatalf("executable content = %q, %v; want %q", got, err, binary)
	}
	info, err := os.Stat(up.ExecPath)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("executable mode = %v, %v; want executable bit set", info.Mode(), err)
	}
	// No staging leftovers next to the binary.
	entries, _ := os.ReadDir(filepath.Dir(up.ExecPath))
	if len(entries) != 1 {
		t.Fatalf("install dir has %d entries, want only the binary", len(entries))
	}
}

func TestApplyAcceptsVersionWithVPrefix(t *testing.T) {
	requirePOSIXShell(t)
	archive := tarGz(t, "ly", fakeBinary("0.2.0"))
	up, _ := testUpdater(t, serveRelease("0.2.0", archive))

	if _, err := up.Apply(context.Background(), "v0.2.0"); err != nil {
		t.Fatalf("Apply(v0.2.0) = %v", err)
	}
}

// A binary that cannot run, or reports a version other than the one we asked
// for, means the release is broken. Keep the working ly rather than install it.
func TestApplyAbortsWhenStagedBinaryFailsSmokeTest(t *testing.T) {
	requirePOSIXShell(t)
	cases := map[string]string{
		"exits non-zero": "#!/bin/sh\nexit 1\n",
		"wrong version":  fakeBinary("0.1.9"),
		"not executable": "this is not a program",
	}
	for name, binary := range cases {
		t.Run(name, func(t *testing.T) {
			archive := tarGz(t, "ly", binary)
			up, _ := testUpdater(t, serveRelease("0.2.0", archive))

			_, err := up.Apply(context.Background(), "0.2.0")
			if err == nil || !strings.Contains(err.Error(), "已保留当前版本") {
				t.Fatalf("Apply() = %v, want smoke-test failure", err)
			}
			got, _ := os.ReadFile(up.ExecPath)
			if string(got) != "old binary" {
				t.Fatalf("executable was replaced by a broken build: %q", got)
			}
		})
	}
}

func TestApplyKeepsChecksumTrustedWhenDirectSourceServesIt(t *testing.T) {
	requirePOSIXShell(t)
	archive := tarGz(t, "ly", fakeBinary("0.2.0"))
	up, _ := testUpdater(t, serveRelease("0.2.0", archive))

	got, err := up.Apply(context.Background(), "0.2.0")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !got.ChecksumTrusted {
		t.Fatalf("ChecksumTrusted = false, want true when the trusted source served checksums")
	}
}

func TestApplyMarksChecksumUntrustedWhenOnlyMirrorServesIt(t *testing.T) {
	requirePOSIXShell(t)
	archive := tarGz(t, "ly", fakeBinary("0.2.0"))

	// 直连全挂：校验和与归档都只能从镜像取，此时必须降级标记。
	up, _ := testUpdater(t, serviceUnavailable())
	mirror := httptest.NewServer(serveRelease("0.2.0", archive))
	defer mirror.Close()
	up.Sources = append(up.Sources, Source{Name: "可用镜像", Base: mirror.URL})

	got, err := up.Apply(context.Background(), "0.2.0")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got.ChecksumTrusted {
		t.Fatalf("ChecksumTrusted = true, want false — 校验和来自不可信镜像时必须降级标记")
	}
	if got.SourceName != "可用镜像" {
		t.Fatalf("SourceName = %q, want %q", got.SourceName, "可用镜像")
	}
}

// 可信源给出的答复是确定性的：它成功返回了 checksums.txt 而其中没有该资产，
// 就说明这个资产不存在。此时若转而采信镜像的另一份 checksums.txt，镜像便可
// 自造资产名与配套归档，用户却只看到一行降级警告。必须直接失败。
func TestApplyRefusesMirrorChecksumWhenTrustedSourceSaysAssetIsAbsent(t *testing.T) {
	requirePOSIXShell(t)
	archive := tarGz(t, "ly", fakeBinary("0.2.0"))

	// 直连正常响应，但 checksums.txt 里只有别的资产。
	up, _ := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
			w.Write([]byte(strings.Repeat("a", 64) + "  ly-some-other-platform.tar.gz\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	mirror := httptest.NewServer(serveRelease("0.2.0", archive))
	defer mirror.Close()
	up.Sources = append(up.Sources, Source{Name: "可用镜像", Base: mirror.URL})

	if _, err := up.Apply(context.Background(), "0.2.0"); err == nil {
		t.Fatal("Apply() error = nil, want failure — 不得改用镜像的校验和")
	}
	got, _ := os.ReadFile(up.ExecPath)
	if string(got) != "old binary" {
		t.Fatalf("executable = %q, want the original left untouched", got)
	}
}

// 代理挂掉时常返回 200 加一张 HTML 错误页。那不是「资产不存在」的确定性答复，
// 只是这个来源不可用，必须继续尝试下一个——否则一个坏代理就能掐断整条回退链。
func TestApplyTreatsUnparsableChecksumBodyAsSourceFailure(t *testing.T) {
	requirePOSIXShell(t)
	archive := tarGz(t, "ly", fakeBinary("0.2.0"))

	up, _ := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
			w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	mirror := httptest.NewServer(serveRelease("0.2.0", archive))
	defer mirror.Close()
	up.Sources = append(up.Sources, Source{Name: "可用镜像", Base: mirror.URL})

	got, err := up.Apply(context.Background(), "0.2.0")
	if err != nil {
		t.Fatalf("Apply() error = %v — 无法解析的响应体应视为来源不可用并换源", err)
	}
	if got.ChecksumTrusted {
		t.Fatalf("ChecksumTrusted = true, want false — 校验和最终来自镜像")
	}
}

func TestApplyAbortsOnChecksumMismatch(t *testing.T) {
	archive := tarGz(t, "ly", "tampered binary")
	asset := testAsset()
	up, _ := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/download/v0.2.0/checksums.txt"):
			// 摘要与实际归档不符，模拟被篡改的包。
			w.Write([]byte(strings.Repeat("a", 64) + "  " + asset + "\n"))
		case strings.HasSuffix(r.URL.Path, "/releases/download/v0.2.0/"+asset):
			w.Write(archive)
		}
	}))

	_, err := up.Apply(context.Background(), "0.2.0")
	if err == nil || !strings.Contains(err.Error(), "校验和不匹配") {
		t.Fatalf("Apply() = %v, want checksum mismatch", err)
	}
	got, _ := os.ReadFile(up.ExecPath)
	if string(got) != "old binary" {
		t.Fatalf("executable was modified despite mismatch: %q", got)
	}
}

func TestApplyFailsWhenArchiveLacksBinary(t *testing.T) {
	archive := tarGz(t, "something-else", "not ly")
	up, _ := testUpdater(t, serveRelease("0.2.0", archive))

	if _, err := up.Apply(context.Background(), "0.2.0"); err == nil {
		t.Fatal("Apply() = nil, want missing-binary error")
	}
	got, _ := os.ReadFile(up.ExecPath)
	if string(got) != "old binary" {
		t.Fatalf("executable was modified: %q", got)
	}
}

func TestApplyFailsWhenInstallDirNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	exec := filepath.Join(dir, "ly")
	if err := os.WriteFile(exec, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	up := &Updater{Current: "0.1.5", ExecPath: exec, HTTP: http.DefaultClient, GOOS: "linux", GOARCH: "amd64"}
	_, err := up.Apply(context.Background(), "0.2.0")
	if err == nil || !strings.Contains(err.Error(), "无法写入") {
		t.Fatalf("Apply() = %v, want unwritable-directory error", err)
	}
}

// ── Binary replacement ──

func TestReplaceExecutableWindowsMovesOldBinaryAside(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ly.exe")
	staged := filepath.Join(dir, ".staged")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(staged, target, true); err != nil {
		t.Fatalf("replaceExecutable() = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "new" {
		t.Fatalf("target = %q, %v; want %q", got, err, "new")
	}
}

// ── Timeouts ──

// A version lookup must not hang for as long as a multi-megabyte download is
// allowed to, so each request carries its own deadline rather than sharing one
// client-level timeout.
func TestLatestVersionAppliesPerRequestDeadline(t *testing.T) {
	release := make(chan struct{})
	up, _ := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	// Deferred here, not via t.Cleanup: the handler must be unblocked before
	// testUpdater's own cleanup calls server.Close, which waits on it.
	defer close(release)

	start := time.Now()
	if _, err := up.LatestVersion(context.Background()); err == nil {
		t.Fatal("LatestVersion() = nil error, want deadline exceeded")
	}
	if elapsed := time.Since(start); elapsed > versionTimeout+5*time.Second {
		t.Fatalf("LatestVersion() blocked for %v, want the %v deadline to apply", elapsed, versionTimeout)
	}
}

func TestValidVersion(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"0.1.5", "0.1.5", true},
		{"v0.1.5", "0.1.5", true},
		{" v1.0.0 ", "1.0.0", true},
		{"1.2.3-rc1", "1.2.3", true},
		{"dev", "", false},
		{"", "", false},
		{"latest", "", false},
		{"1.x.3", "", false},
	}
	for _, tc := range cases {
		got, err := validVersion(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("validVersion(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("validVersion(%q) = %q, nil; want error", tc.in, got)
		}
	}
}
