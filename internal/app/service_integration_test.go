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
)

// TestServiceAgainstFakeRegistry 用一个假的 registry 验证服务层的完整链路：
// 配置加载 → 客户端创建 → 列仓库 → 列 tag → 取清单。
func TestServiceAgainstFakeRegistry(t *testing.T) {
	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"size":      7023,
			"digest":    "sha256:config",
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"size":      4096,
				"digest":    "sha256:layer",
			},
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	sum := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
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
			_, _ = w.Write([]byte(`{"name":"team/app","tags":["2.2.6","latest"]}`))
		case strings.HasSuffix(path, "/manifests/2.2.6"):
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Docker-Content-Digest", digest)
			_, _ = w.Write(body)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")

	dataDir := t.TempDir()
	toolConfig := fmt.Sprintf(`
[[registry]]
name = "假仓库"
host = %q
insecure = true
`, host)
	if err := os.WriteFile(filepath.Join(dataDir, "registries.toml"), []byte(toolConfig), 0o600); err != nil {
		t.Fatalf("write tool config: %v", err)
	}
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", dataDir)
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "absent"))

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	registries, err := service.ListRegistries()
	if err != nil {
		t.Fatalf("ListRegistries: %v", err)
	}
	if len(registries) != 1 || registries[0].Host != host {
		t.Fatalf("unexpected registries: %+v", registries)
	}

	repositories, err := service.ListRepositories(host)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 || repositories[0].Name != "team/app" {
		t.Fatalf("unexpected repositories: %+v", repositories)
	}
	if repositories[0].Namespace != "team" || repositories[0].Image != "app" {
		t.Errorf("unexpected namespace/image: %q/%q", repositories[0].Namespace, repositories[0].Image)
	}

	tags, err := service.ListTags(host, "team/app")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 2 || tags[0].Name != "2.2.6" {
		t.Fatalf("unexpected tags: %+v", tags)
	}

	info, err := service.GetManifest(host, "team/app", "2.2.6")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if info.Digest != digest {
		t.Errorf("Digest = %q, want %q", info.Digest, digest)
	}
	if len(info.Layers) != 1 || info.Layers[0].Size != 4096 {
		t.Errorf("unexpected layers: %+v", info.Layers)
	}
}

func TestServiceUnknownRegistry(t *testing.T) {
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", t.TempDir())
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "absent"))

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := service.ListRepositories("missing.example.com"); err == nil {
		t.Fatal("expected an error for an unknown registry")
	}
}

func TestServiceParseReference(t *testing.T) {
	t.Setenv("REGISTRY_BROWSER_DATA_DIR", t.TempDir())
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "absent"))

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ref, err := service.ParseReference("higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one:2.2.6")
	if err != nil {
		t.Fatalf("ParseReference: %v", err)
	}
	if ref.Registry != "higress-registry.cn-hangzhou.cr.aliyuncs.com" {
		t.Errorf("Registry = %q", ref.Registry)
	}
	if ref.Namespace != "higress" || ref.Image != "all-in-one" || ref.Tag != "2.2.6" {
		t.Errorf("unexpected reference: %+v", ref)
	}

	if _, err := service.ParseReference("Library/Nginx"); err == nil {
		t.Fatal("expected an error for an invalid reference")
	}
}
