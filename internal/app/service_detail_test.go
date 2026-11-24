package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ch3cho/registry-browser/internal/registry"
)

// detailRegistry 是一个支持清单、配置 blob 与标签列举的假 registry。
type detailRegistry struct {
	// config 是返回给 image config 请求的内容。
	config []byte
	// manifestStatus 非 0 时，清单请求返回该状态码。
	manifestStatus int
	// blobStatus 非 0 时，配置 blob 请求返回该状态码。
	blobStatus int
}

func (d *detailRegistry) handler() http.Handler {
	mux := http.NewServeMux()

	configDigest := ""
	if len(d.config) > 0 {
		sum := sha256.Sum256(d.config)
		configDigest = "sha256:" + hex.EncodeToString(sum[:])
	}

	layerDigest := "sha256:" + strings.Repeat("ab", 32)

	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"size":      len(d.config),
			"digest":    configDigest,
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"size":      2048,
				"digest":    layerDigest,
			},
		},
	}
	manifestBody, _ := json.Marshal(manifest)

	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v2/")
		switch {
		case path == "":
			_, _ = w.Write([]byte(`{}`))
		case path == "_catalog":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repositories":["team/app"]}`))
		case path == "team/app/tags/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"team/app","tags":["1.0.0","latest"]}`))
		case strings.Contains(path, "/manifests/"):
			if d.manifestStatus != 0 {
				http.Error(w, "boom", d.manifestStatus)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			_, _ = w.Write(manifestBody)
		case strings.Contains(path, "/blobs/"):
			if d.blobStatus != 0 {
				http.Error(w, "boom", d.blobStatus)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.config.v1+json")
			_, _ = w.Write(d.config)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	return mux
}

// newDetailService 起一个假 registry，并让服务指向它。
func newDetailService(t *testing.T, fake *detailRegistry) (*Service, string) {
	t.Helper()

	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	dataDir := t.TempDir()
	toolConfig := fmt.Sprintf("[[registry]]\nname = \"假仓库\"\nhost = %q\ninsecure = true\n", host)
	if err := os.WriteFile(filepath.Join(dataDir, "registries.toml"), []byte(toolConfig), 0o600); err != nil {
		t.Fatalf("write tool config: %v", err)
	}

	t.Setenv("REGISTRY_BROWSER_DATA_DIR", dataDir)
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "absent"))

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, host
}

func imageConfigBlob() []byte {
	return []byte(`{
  "architecture": "amd64",
  "os": "linux",
  "created": "2026-02-03T04:05:06Z",
  "config": {"Labels": {"org.opencontainers.image.title": "demo"}}
}`)
}

func TestListNamespaces(t *testing.T) {
	service, host := newDetailService(t, &detailRegistry{config: imageConfigBlob()})

	namespaces, err := service.ListNamespaces(host)
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(namespaces) != 1 {
		t.Fatalf("expected 1 namespace, got %+v", namespaces)
	}
	if namespaces[0].Name != "team" {
		t.Errorf("Name = %q, want %q", namespaces[0].Name, "team")
	}
	if len(namespaces[0].Images) != 1 || namespaces[0].Images[0].Name != "app" {
		t.Errorf("unexpected images: %+v", namespaces[0].Images)
	}
	if namespaces[0].Images[0].Repository != "team/app" {
		t.Errorf("Repository = %q", namespaces[0].Images[0].Repository)
	}
}

func TestListNamespacesUnknownRegistry(t *testing.T) {
	service, _ := newDetailService(t, &detailRegistry{config: imageConfigBlob()})

	_, err := service.ListNamespaces("missing.example.com")
	if err == nil {
		t.Fatal("expected an error for an unknown registry")
	}
}

func TestGetTagDetail(t *testing.T) {
	service, host := newDetailService(t, &detailRegistry{config: imageConfigBlob()})

	detail, err := service.GetTagDetail(host, "team/app", "1.0.0")
	if err != nil {
		t.Fatalf("GetTagDetail: %v", err)
	}
	if detail.Manifest == nil {
		t.Fatal("expected manifest details")
	}
	if detail.Manifest.MediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Errorf("MediaType = %q", detail.Manifest.MediaType)
	}
	if len(detail.Manifest.Layers) != 1 || detail.Manifest.Layers[0].Size != 2048 {
		t.Errorf("unexpected layers: %+v", detail.Manifest.Layers)
	}
	if detail.Config == nil {
		t.Fatal("expected image config details")
	}
	if detail.Config.OS != "linux" || detail.Config.Architecture != "amd64" {
		t.Errorf("unexpected platform: %s/%s", detail.Config.OS, detail.Config.Architecture)
	}
	if detail.Config.Labels["org.opencontainers.image.title"] != "demo" {
		t.Errorf("unexpected labels: %v", detail.Config.Labels)
	}
}

// 镜像配置取不到时，清单仍然要能展示。
func TestGetTagDetailWithoutImageConfig(t *testing.T) {
	service, host := newDetailService(t, &detailRegistry{
		config:     imageConfigBlob(),
		blobStatus: http.StatusNotFound,
	})

	detail, err := service.GetTagDetail(host, "team/app", "1.0.0")
	if err != nil {
		t.Fatalf("GetTagDetail: %v", err)
	}
	if detail.Manifest == nil {
		t.Fatal("expected manifest details even when the image config is unavailable")
	}
	if detail.Config != nil {
		t.Errorf("expected no image config, got %+v", detail.Config)
	}
}

func TestGetTagDetailManifestError(t *testing.T) {
	service, host := newDetailService(t, &detailRegistry{
		config:         imageConfigBlob(),
		manifestStatus: http.StatusNotFound,
	})

	_, err := service.GetTagDetail(host, "team/app", "1.0.0")
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Kind != "not_found" {
		t.Errorf("Kind = %q, want %q", apiErr.Kind, "not_found")
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d", apiErr.Status)
	}
}

func TestToAPIError(t *testing.T) {
	apiErr := toAPIError(registry.AsError(fmt.Errorf("boom"))).(*APIError)
	if apiErr.Kind != string(registry.KindNetwork) {
		t.Errorf("Kind = %q, want %q", apiErr.Kind, registry.KindNetwork)
	}
	if apiErr.Message == "" {
		t.Error("expected a non-empty message")
	}
}

func TestDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", dir)

	resolved, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if resolved != dir {
		t.Errorf("DataDir() = %q, want %q", resolved, dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("expected the data directory to be created: %v", err)
	}
}

func TestToolConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", dir)

	path, err := ToolConfigPath()
	if err != nil {
		t.Fatalf("ToolConfigPath: %v", err)
	}
	if want := filepath.Join(dir, "registries.toml"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

func TestLoadToolConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "registries.toml"), []byte("这不是 TOML = = ="), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", dir)
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "absent"))

	if _, err := LoadSettings(); err == nil {
		t.Fatal("expected an error for a malformed tool config")
	}
}

func TestLoadSettingsMalformedDockerConfig(t *testing.T) {
	dockerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("DOCKER_CONFIG", dockerDir)
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", t.TempDir())

	if _, err := LoadSettings(); err == nil {
		t.Fatal("expected an error for a malformed docker config")
	}
}
