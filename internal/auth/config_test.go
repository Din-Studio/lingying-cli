package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathForUsesPlatformConventions(t *testing.T) {
	if got := configPathFor("/home/alice", "", "linux"); got != "/home/alice/.config/ly/config.json" {
		t.Fatalf("linux path = %q", got)
	}
	if got := configPathFor("/Users/alice", "", "darwin"); got != "/Users/alice/.config/ly/config.json" {
		t.Fatalf("darwin path = %q", got)
	}
	if got := configPathFor(`C:\Users\Alice`, `C:\Users\Alice\AppData\Roaming`, "windows"); got != `C:\Users\Alice\AppData\Roaming\ly\config.json` {
		t.Fatalf("windows path = %q", got)
	}
}

func TestStoreAPIKeyWritesVersionedPrivateConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LY_CONFIG_FILE", "")
	if err := StoreAPIKey("secret-key"); err != nil {
		t.Fatalf("StoreAPIKey() error = %v", err)
	}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || !contains(string(data), `"version": 1`) || !contains(string(data), `"api_key": "secret-key"`) {
		t.Fatalf("config = %s", data)
	}
	if mode := mustStat(t, ConfigPath()).Mode().Perm(); mode != 0600 {
		t.Fatalf("config permission = %o, want 600", mode)
	}
	if filepath.Dir(ConfigPath()) != filepath.Join(home, ".config", "ly") {
		t.Fatalf("config directory = %q", filepath.Dir(ConfigPath()))
	}
}

func TestConfigPathHonorsExplicitOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-config.json")
	t.Setenv("LY_CONFIG_FILE", path)
	if got := ConfigPath(); got != path {
		t.Fatalf("ConfigPath() = %q, want %q", got, path)
	}
}

func TestClearCredentialsRetainsNonCredentialSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("LY_CONFIG_FILE", path)
	t.Setenv("LY_ACCESS_TOKEN", "")
	t.Setenv("LY_API_KEY", "")
	if err := StoreOAuthToken("access-token", "refresh-token"); err != nil {
		t.Fatal(err)
	}
	if err := StoreOutputDir("/tmp/ly-output"); err != nil {
		t.Fatal(err)
	}
	if err := ClearCredentials(); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(); got.Value != "" {
		t.Fatalf("Resolve() = %#v, want no credentials", got)
	}
	if got := GetOutputDir(); got != "/tmp/ly-output" {
		t.Fatalf("GetOutputDir() = %q", got)
	}
}

func contains(s, fragment string) bool {
	return len(s) >= len(fragment) && (s == fragment || containsAt(s, fragment))
}
func containsAt(s, fragment string) bool {
	for i := 0; i+len(fragment) <= len(s); i++ {
		if s[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
