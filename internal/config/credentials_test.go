package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDockerConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	basic := base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	content := `{
  "auths": {
    "registry.example.com": { "auth": "` + basic + `" },
    "identity.example.com": { "identitytoken": "abc.def.ghi" },
    "legacy.example.com": { "username": "bob", "password": "pw" },
    "https://index.docker.io/v1/": { "auth": "` + basic + `" }
  },
  "credsStore": "desktop",
  "credHelpers": { "helper.example.com": "gcloud" }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadDockerConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.CredsStore != "desktop" {
		t.Errorf("CredsStore = %q, want %q", cfg.CredsStore, "desktop")
	}
	if got := cfg.CredHelpers["helper.example.com"]; got != "gcloud" {
		t.Errorf("credHelpers[helper.example.com] = %q, want %q", got, "gcloud")
	}

	cred, ok := cfg.Credentials("registry.example.com")
	if !ok {
		t.Fatal("expected credentials for registry.example.com")
	}
	if cred.Username != "alice" || cred.Password != "s3cret" {
		t.Errorf("unexpected username/password: %q/%q", cred.Username, cred.Password)
	}

	cred, ok = cfg.Credentials("identity.example.com")
	if !ok {
		t.Fatal("expected credentials for identity.example.com")
	}
	if cred.IdentityToken != "abc.def.ghi" {
		t.Errorf("IdentityToken = %q, want %q", cred.IdentityToken, "abc.def.ghi")
	}

	cred, ok = cfg.Credentials("legacy.example.com")
	if !ok {
		t.Fatal("expected credentials for legacy.example.com")
	}
	if cred.Username != "bob" || cred.Password != "pw" {
		t.Errorf("unexpected username/password: %q/%q", cred.Username, cred.Password)
	}

	// Docker Hub 的历史键名需要归一化到 docker.io。
	if _, ok := cfg.Credentials("docker.io"); !ok {
		t.Error("expected credentials to be reachable under docker.io")
	}
	if _, ok := cfg.Credentials("index.docker.io"); !ok {
		t.Error("expected credentials to be reachable under index.docker.io")
	}
}

func TestLoadDockerConfigMissingFile(t *testing.T) {
	cfg, err := LoadDockerConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("missing file should not be an error: %v", err)
	}
	if len(cfg.Auths) != 0 {
		t.Errorf("expected no auths, got %d", len(cfg.Auths))
	}
}

func TestLoadDockerConfigInvalidBase64(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"auths": {"registry.example.com": {"auth": "not-base64!!!"}}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadDockerConfig(path); err == nil {
		t.Fatal("expected an error for invalid base64 auth")
	}
}

func TestLoadDockerConfigMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadDockerConfig(path); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestCredentialHasSecret(t *testing.T) {
	cases := []struct {
		name string
		cred Credential
		want bool
	}{
		{"username and password", Credential{Username: "a", Password: "b"}, true},
		{"identity token only", Credential{IdentityToken: "t"}, true},
		{"password only", Credential{Password: "b"}, true},
		{"empty", Credential{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cred.HasSecret(); got != tc.want {
				t.Errorf("HasSecret() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDockerConfigHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	basic := base64.StdEncoding.EncodeToString([]byte("a:b"))
	content := `{
  "auths": {
    "registry.example.com": { "auth": "` + basic + `" },
    "https://index.docker.io/v1/": { "auth": "` + basic + `" },
    "docker.io": { "auth": "` + basic + `" }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadDockerConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hosts := cfg.Hosts()
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts after normalization, got %v", hosts)
	}
	found := map[string]bool{}
	for _, h := range hosts {
		found[h] = true
	}
	if !found["registry.example.com"] {
		t.Error("missing registry.example.com")
	}
	if !found["docker.io"] {
		t.Error("missing normalized docker.io")
	}
}

func TestDefaultDockerConfigPath(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", filepath.Join("custom", "docker"))
	got, err := DefaultDockerConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join("custom", "docker", "config.json"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	t.Setenv("DOCKER_CONFIG", "")
	got, err = DefaultDockerConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	if want := filepath.Join(home, ".docker", "config.json"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}
