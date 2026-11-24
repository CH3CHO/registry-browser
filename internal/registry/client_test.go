package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRegistry 是一个最小可用的 Distribution v2 假服务，用于验证分页、标签、清单与错误分类。
type fakeRegistry struct {
	repos     []string
	tags      map[string][]string
	manifests map[string][]byte
	// status 覆盖特定路径前缀返回的 HTTP 状态码。
	status map[string]int
}

func newFakeRegistry(t *testing.T, fake *fakeRegistry) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		for prefix, code := range fake.status {
			if strings.HasPrefix(r.URL.Path, prefix) {
				http.Error(w, http.StatusText(code), code)
				return
			}
		}

		path := strings.TrimPrefix(r.URL.Path, "/v2/")

		switch {
		case path == "":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case path == "_catalog":
			repos := paginate(fake.repos, r.URL.Query().Get("last"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": repos})
		case strings.HasSuffix(path, "/tags/list"):
			repo := strings.TrimSuffix(path, "/tags/list")
			tags, ok := fake.tags[repo]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": repo, "tags": tags})
		case strings.Contains(path, "/manifests/"):
			idx := strings.LastIndex(path, "/manifests/")
			repo, ref := path[:idx], path[idx+len("/manifests/"):]
			body, ok := fake.manifests[repo+":"+ref]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", manifestContentType(body))
			w.Header().Set("Docker-Content-Digest", digestOf(body))
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// paginate 模拟 registry 的 last/n 分页语义。
func paginate(items []string, last string) []string {
	start := 0
	if last != "" {
		for i, item := range items {
			if item == last {
				start = i + 1
				break
			}
		}
	}
	return items[start:]
}

func digestOf(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func manifestContentType(body []byte) string {
	var probe struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.MediaType == "" {
		return "application/vnd.oci.image.manifest.v1+json"
	}
	return probe.MediaType
}

func imageManifest(t *testing.T, layerSize int) []byte {
	t.Helper()
	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"size":      7023,
			"digest":    digestOf([]byte("config")),
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"size":      layerSize,
				"digest":    digestOf([]byte("layer-1")),
			},
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return body
}

func indexManifest(t *testing.T) []byte {
	t.Helper()
	child := imageManifest(t, 100)
	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.index.v1+json",
		"manifests": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"size":      len(child),
				"digest":    digestOf(child),
				"platform":  map[string]any{"architecture": "amd64", "os": "linux"},
			},
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"size":      len(child),
				"digest":    digestOf(child),
				"platform":  map[string]any{"architecture": "arm64", "os": "linux"},
			},
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	return body
}

func newTestClient(t *testing.T, server *httptest.Server, pageSize int) *Client {
	t.Helper()
	host := strings.TrimPrefix(server.URL, "http://")
	client, err := New(Config{
		Host:     host,
		Insecure: true,
		PageSize: pageSize,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func TestListRepositories(t *testing.T) {
	fake := &fakeRegistry{
		repos: []string{"team/a", "team/b", "team/c", "team/d", "other/e"},
	}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 2)
	repos, err := client.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	want := []string{"team/a", "team/b", "team/c", "team/d", "other/e"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos %v, want %d %v", len(repos), repos, len(want), want)
	}
	for i := range want {
		if repos[i] != want[i] {
			t.Fatalf("repos[%d] = %q, want %q", i, repos[i], want[i])
		}
	}
}

func TestListRepositoriesUnauthorized(t *testing.T) {
	fake := &fakeRegistry{status: map[string]int{"/v2/_catalog": http.StatusUnauthorized}}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	_, err := client.ListRepositories(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	regErr := AsError(err)
	if regErr.Kind != KindUnauthorized {
		t.Errorf("Kind = %q, want %q", regErr.Kind, KindUnauthorized)
	}
	if regErr.UserHint() == "" {
		t.Error("expected a user hint for unauthorized")
	}
}

func TestListRepositoriesForbidden(t *testing.T) {
	fake := &fakeRegistry{status: map[string]int{"/v2/_catalog": http.StatusForbidden}}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	_, err := client.ListRepositories(context.Background())
	regErr := AsError(err)
	if regErr.Kind != KindForbidden {
		t.Errorf("Kind = %q, want %q", regErr.Kind, KindForbidden)
	}
}

// 公共 registry（Docker Hub 等）通常不实现 /v2/_catalog，会以 400/404/405/501 回应。
// 这些都应归类为「不支持」，让上层降级为手输镜像地址。
func TestListRepositoriesUnsupported(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotImplemented,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			fake := &fakeRegistry{status: map[string]int{"/v2/_catalog": status}}
			server := newFakeRegistry(t, fake)

			client := newTestClient(t, server, 10)
			_, err := client.ListRepositories(context.Background())
			regErr := AsError(err)
			if regErr.Kind != KindUnsupported {
				t.Errorf("Kind = %q, want %q", regErr.Kind, KindUnsupported)
			}
		})
	}
}

func TestListTags(t *testing.T) {
	fake := &fakeRegistry{
		tags: map[string][]string{
			"team/app": {"2.2.6", "latest", "v1.0.0"},
		},
	}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	tags, err := client.ListTags(context.Background(), "team/app")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 3 || tags[0] != "2.2.6" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestListTagsNotFound(t *testing.T) {
	fake := &fakeRegistry{tags: map[string][]string{}}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	_, err := client.ListTags(context.Background(), "missing/app")
	regErr := AsError(err)
	if regErr.Kind != KindNotFound {
		t.Errorf("Kind = %q, want %q", regErr.Kind, KindNotFound)
	}
}

func TestGetManifest(t *testing.T) {
	body := imageManifest(t, 1024)
	fake := &fakeRegistry{
		manifests: map[string][]byte{"team/app:2.2.6": body},
	}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	manifest, err := client.GetManifest(context.Background(), "team/app", "2.2.6")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if manifest.Digest != digestOf(body) {
		t.Errorf("Digest = %q, want %q", manifest.Digest, digestOf(body))
	}
	if len(manifest.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(manifest.Layers))
	}
	if manifest.Layers[0].Size != 1024 {
		t.Errorf("layer size = %d, want 1024", manifest.Layers[0].Size)
	}
	if manifest.Config == nil {
		t.Fatal("expected config descriptor")
	}
	if manifest.Config.Size != 7023 {
		t.Errorf("config size = %d, want 7023", manifest.Config.Size)
	}
	if len(manifest.Platforms) != 0 {
		t.Errorf("expected no platforms for a single-arch manifest, got %d", len(manifest.Platforms))
	}
}

func TestGetManifestMultiArch(t *testing.T) {
	body := indexManifest(t)
	fake := &fakeRegistry{
		manifests: map[string][]byte{"team/app:latest": body},
	}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	manifest, err := client.GetManifest(context.Background(), "team/app", "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(manifest.Platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(manifest.Platforms))
	}
	if manifest.Platforms[0].Platform != "linux/amd64" {
		t.Errorf("platform[0] = %q, want linux/amd64", manifest.Platforms[0].Platform)
	}
	if manifest.Platforms[1].Platform != "linux/arm64" {
		t.Errorf("platform[1] = %q, want linux/arm64", manifest.Platforms[1].Platform)
	}
}

func TestGetManifestNotFound(t *testing.T) {
	fake := &fakeRegistry{manifests: map[string][]byte{}}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	_, err := client.GetManifest(context.Background(), "team/app", "nope")
	regErr := AsError(err)
	if regErr.Kind != KindNotFound {
		t.Errorf("Kind = %q, want %q", regErr.Kind, KindNotFound)
	}
}

func TestBasePathIsPrefixed(t *testing.T) {
	// Artifactory 之类的 registry 需要 /artifactory/api/docker/<repoKey> 前缀。
	var seenPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/docker/team/v2/_catalog", func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"repositories":["team/app"]}`))
	})
	mux.HandleFunc("/artifactory/api/docker/team/v2/", func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	client, err := New(Config{
		Host:     host,
		BasePath: "/artifactory/api/docker/team",
		Insecure: true,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	repos, err := client.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0] != "team/app" {
		t.Fatalf("unexpected repos: %v", repos)
	}
	if !strings.HasPrefix(seenPath, "/artifactory/api/docker/team/v2/") {
		t.Errorf("request path = %q, want the base path to be prefixed", seenPath)
	}
}
