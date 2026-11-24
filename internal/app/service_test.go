package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitRepository(t *testing.T) {
	cases := []struct {
		repository string
		namespace  string
		image      string
	}{
		{"team/app", "team", "app"},
		{"app", "", "app"},
		{"team/sub/app", "team/sub", "app"},
		{"a/b/c/d", "a/b/c", "d"},
	}

	for _, tc := range cases {
		t.Run(tc.repository, func(t *testing.T) {
			namespace, image := splitRepository(tc.repository)
			if namespace != tc.namespace || image != tc.image {
				t.Errorf("splitRepository(%q) = %q/%q, want %q/%q", tc.repository, namespace, image, tc.namespace, tc.image)
			}
		})
	}
}

func TestSettingsFind(t *testing.T) {
	settings := &Settings{Registries: []RegistryEntry{
		{Host: "a.example.com"},
		{Host: "b.example.com"},
	}}

	entry, ok := settings.Find("b.example.com")
	if !ok {
		t.Fatal("expected to find b.example.com")
	}
	if entry.Host != "b.example.com" {
		t.Errorf("Host = %q", entry.Host)
	}
	if _, ok := settings.Find("missing"); ok {
		t.Error("did not expect to find a missing host")
	}
}

func TestRegistryEntryDisplayName(t *testing.T) {
	if got := (RegistryEntry{Host: "h"}).DisplayName(); got != "h" {
		t.Errorf("DisplayName() = %q, want %q", got, "h")
	}
	if got := (RegistryEntry{Host: "h", Name: "别名"}).DisplayName(); got != "别名" {
		t.Errorf("DisplayName() = %q, want %q", got, "别名")
	}
}

func TestLoadSettingsMergesToolAndDockerConfig(t *testing.T) {
	dir := t.TempDir()

	dockerDir := filepath.Join(dir, "docker")
	if err := os.MkdirAll(dockerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	basic := base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	dockerConfig := `{
  "auths": {
    "tool.example.com": { "auth": "` + basic + `" },
    "docker-only.example.com": { "auth": "` + basic + `" }
  }
}`
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(dockerConfig), 0o600); err != nil {
		t.Fatalf("write docker config: %v", err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)

	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	toolConfig := `
[[registry]]
name = "工具仓库"
host = "tool.example.com"
namespace = "higress"
base_path = "/artifactory/api/docker/team"

[[registry]]
name = "本地"
host = "local.example.com"
insecure = true
skip_tls_verify = true
`
	if err := os.WriteFile(filepath.Join(dataDir, "registries.toml"), []byte(toolConfig), 0o600); err != nil {
		t.Fatalf("write tool config: %v", err)
	}
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", dataDir)

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	byHost := map[string]RegistryEntry{}
	for _, entry := range settings.Registries {
		byHost[entry.Host] = entry
	}

	tool, ok := byHost["tool.example.com"]
	if !ok {
		t.Fatal("expected tool.example.com to be present")
	}
	if tool.Source != "tool" {
		t.Errorf("Source = %q, want %q", tool.Source, "tool")
	}
	if tool.Namespace != "higress" {
		t.Errorf("Namespace = %q, want %q", tool.Namespace, "higress")
	}
	if tool.BasePath != "/artifactory/api/docker/team" {
		t.Errorf("BasePath = %q", tool.BasePath)
	}
	if !tool.HasCredential {
		t.Error("expected credentials to be picked up from the docker config")
	}
	if tool.Credential.Username != "alice" || tool.Credential.Password != "s3cret" {
		t.Errorf("unexpected credentials: %q/%q", tool.Credential.Username, tool.Credential.Password)
	}

	local, ok := byHost["local.example.com"]
	if !ok {
		t.Fatal("expected local.example.com to be present")
	}
	if !local.Insecure || !local.SkipTLSVerify {
		t.Errorf("expected insecure/skipTLSVerify to be carried over: %+v", local)
	}
	if local.HasCredential {
		t.Error("expected no credentials for local.example.com")
	}

	only, ok := byHost["docker-only.example.com"]
	if !ok {
		t.Fatal("expected docker-only.example.com to be present")
	}
	if only.Source != "docker" {
		t.Errorf("Source = %q, want %q", only.Source, "docker")
	}
}

func TestLoadSettingsWithoutToolConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", filepath.Join(dir, "absent"))
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", filepath.Join(dir, "data"))

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if len(settings.Registries) != 0 {
		t.Errorf("expected no registries, got %d", len(settings.Registries))
	}
}

func TestAPIErrorMessage(t *testing.T) {
	err := &APIError{Kind: "unauthorized", Message: "list tags failed", Hint: "请执行 docker login"}
	if got := err.Error(); got != "list tags failed — 请执行 docker login" {
		t.Errorf("Error() = %q", got)
	}

	plain := &APIError{Kind: "network", Message: "connection refused"}
	if got := plain.Error(); got != "connection refused" {
		t.Errorf("Error() = %q", got)
	}
}
