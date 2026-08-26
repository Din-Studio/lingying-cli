package updater

import (
	"os"
	"testing"
)

func TestAssetURLEmptyVersionResolvesToLatest(t *testing.T) {
	s := Source{Base: "https://github.com"}
	got := s.AssetURL("Din-Studio/lingying-cli", "", "ly-darwin-arm64.tar.gz")
	want := "https://github.com/Din-Studio/lingying-cli/releases/latest/download/ly-darwin-arm64.tar.gz"
	if got != want {
		t.Fatalf("AssetURL() = %q, want %q", got, want)
	}
}

func TestAssetURLPinnedVersionNormalizesLeadingV(t *testing.T) {
	s := Source{Base: "https://github.com"}
	want := "https://github.com/Din-Studio/lingying-cli/releases/download/v0.1.6/ly-linux-amd64.tar.gz"
	for _, version := range []string{"0.1.6", "v0.1.6"} {
		if got := s.AssetURL("Din-Studio/lingying-cli", version, "ly-linux-amd64.tar.gz"); got != want {
			t.Fatalf("AssetURL(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestLatestTagURLPointsAtTheRedirectingEndpoint(t *testing.T) {
	s := Source{Base: "https://github.com"}
	got := s.LatestTagURL("Din-Studio/lingying-cli")
	want := "https://github.com/Din-Studio/lingying-cli/releases/latest"
	if got != want {
		t.Fatalf("LatestTagURL() = %q, want %q", got, want)
	}
}

func TestSourcesPutDirectFirstAndMirrorBeforePublicProxies(t *testing.T) {
	t.Setenv("LY_MIRROR", "https://mirror.example.com/")
	got := Sources()
	if len(got) != 4 {
		t.Fatalf("len(Sources()) = %d, want 4", len(got))
	}
	if !got[0].Trusted || got[0].Base != "https://github.com" {
		t.Fatalf("Sources()[0] = %+v, want the trusted direct source", got[0])
	}
	if got[1].Base != "https://mirror.example.com/https://github.com" {
		t.Fatalf("Sources()[1].Base = %q, want the LY_MIRROR value with the trailing slash trimmed", got[1].Base)
	}
	if got[1].Trusted {
		t.Fatalf("LY_MIRROR source must not be marked trusted — 镜像可同时替换包与校验和")
	}
}

func TestSourcesOmitMirrorWhenUnset(t *testing.T) {
	_ = os.Unsetenv("LY_MIRROR")
	got := Sources()
	if len(got) != 3 {
		t.Fatalf("len(Sources()) = %d, want 3", len(got))
	}
	for i, s := range got[1:] {
		if s.Trusted {
			t.Fatalf("Sources()[%d] = %+v, want public proxies marked untrusted", i+1, s)
		}
	}
}
