package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1"
)

// configBlob 是一个最小的 OCI image config。
func configBlob() []byte {
	blob := map[string]any{
		"architecture": "amd64",
		"os":           "linux",
		"created":      "2026-01-02T03:04:05Z",
		"author":       "registry-browser",
		"config": map[string]any{
			"Labels":     map[string]string{"org.opencontainers.image.title": "demo"},
			"Env":        []string{"PATH=/usr/bin"},
			"Entrypoint": []string{"/entrypoint.sh"},
			"Cmd":        []string{"--serve"},
			"User":       "app",
			"WorkingDir": "/work",
			"ExposedPorts": map[string]struct{}{
				"8080/tcp": {},
			},
		},
		"history": []map[string]any{
			{"created": "2026-01-02T03:04:00Z", "created_by": "RUN echo hi", "comment": "step"},
			{"created": "2026-01-02T03:04:05Z", "created_by": "CMD [\"/bin/sh\"]", "empty_layer": true},
		},
	}
	body, err := json.Marshal(blob)
	if err != nil {
		panic(err)
	}
	return body
}

func TestGetImageConfig(t *testing.T) {
	blob := configBlob()
	sum := sha256.Sum256(blob)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"size":      len(blob),
			"digest":    digest,
		},
		"layers": []map[string]any{},
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v2/")
		switch {
		case path == "":
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(path, "/manifests/"):
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			_, _ = w.Write(manifestBody)
		case strings.Contains(path, "/blobs/"):
			w.Header().Set("Content-Type", "application/vnd.oci.image.config.v1+json")
			_, _ = w.Write(blob)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server, 10)
	imageConfig, err := client.GetImageConfig(context.Background(), "team/app", "2.2.6")
	if err != nil {
		t.Fatalf("GetImageConfig: %v", err)
	}

	if imageConfig.OS != "linux" || imageConfig.Architecture != "amd64" {
		t.Errorf("unexpected platform: %s/%s", imageConfig.OS, imageConfig.Architecture)
	}
	if imageConfig.Created != "2026-01-02T03:04:05Z" {
		t.Errorf("Created = %q", imageConfig.Created)
	}
	if imageConfig.Author != "registry-browser" {
		t.Errorf("Author = %q", imageConfig.Author)
	}
	if imageConfig.Labels["org.opencontainers.image.title"] != "demo" {
		t.Errorf("unexpected labels: %v", imageConfig.Labels)
	}
	if len(imageConfig.Env) != 1 || imageConfig.Env[0] != "PATH=/usr/bin" {
		t.Errorf("unexpected env: %v", imageConfig.Env)
	}
	if len(imageConfig.Entrypoint) != 1 || imageConfig.Entrypoint[0] != "/entrypoint.sh" {
		t.Errorf("unexpected entrypoint: %v", imageConfig.Entrypoint)
	}
	if imageConfig.User != "app" || imageConfig.WorkingDir != "/work" {
		t.Errorf("unexpected user/workingDir: %q/%q", imageConfig.User, imageConfig.WorkingDir)
	}
	if _, ok := imageConfig.ExposedPorts["8080/tcp"]; !ok {
		t.Errorf("unexpected exposed ports: %v", imageConfig.ExposedPorts)
	}
	if len(imageConfig.History) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(imageConfig.History))
	}
	if imageConfig.History[0].CreatedBy != "RUN echo hi" {
		t.Errorf("history[0].CreatedBy = %q", imageConfig.History[0].CreatedBy)
	}
	if !imageConfig.History[1].EmptyLayer {
		t.Error("history[1] should be an empty layer")
	}
}

func TestGetImageConfigMultiArch(t *testing.T) {
	blob := configBlob()
	sum := sha256.Sum256(blob)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	child := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"size":      len(blob),
			"digest":    digest,
		},
		"layers": []map[string]any{},
	}
	childBody, err := json.Marshal(child)
	if err != nil {
		t.Fatalf("marshal child: %v", err)
	}

	index := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.index.v1+json",
		"manifests": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"size":      len(childBody),
				"digest":    digestOf(childBody),
				"platform":  map[string]any{"architecture": "arm64", "os": "linux"},
			},
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"size":      len(childBody),
				"digest":    digestOf(childBody),
				"platform":  map[string]any{"architecture": "amd64", "os": "linux"},
			},
		},
	}
	indexBody, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v2/")
		switch {
		case path == "":
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(path, "/manifests/"):
			ref := path[strings.LastIndex(path, "/")+1:]
			if strings.HasPrefix(ref, "sha256:") {
				// 子清单按摘要拉取。
				w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
				_, _ = w.Write(childBody)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			_, _ = w.Write(indexBody)
		case strings.Contains(path, "/blobs/"):
			w.Header().Set("Content-Type", "application/vnd.oci.image.config.v1+json")
			_, _ = w.Write(blob)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server, 10)
	imageConfig, err := client.GetImageConfig(context.Background(), "team/app", "latest")
	if err != nil {
		t.Fatalf("GetImageConfig: %v", err)
	}
	if imageConfig.Platform != "linux/amd64" {
		t.Errorf("Platform = %q, want linux/amd64", imageConfig.Platform)
	}
}

func TestGetImageConfigNotFound(t *testing.T) {
	fake := &fakeRegistry{manifests: map[string][]byte{}}
	server := newFakeRegistry(t, fake)

	client := newTestClient(t, server, 10)
	_, err := client.GetImageConfig(context.Background(), "team/app", "missing")
	regErr := AsError(err)
	if regErr.Kind != KindNotFound {
		t.Errorf("Kind = %q, want %q", regErr.Kind, KindNotFound)
	}
}

func TestConvertConfigNil(t *testing.T) {
	if got := convertConfig(nil, "linux/amd64"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestPlatformString(t *testing.T) {
	cases := []struct {
		platform string
		want     string
	}{
		{"linux/amd64", "linux/amd64"},
		{"linux/arm64/v8", "linux/arm64/v8"},
		{"", ""},
	}
	for _, tc := range cases {
		parts := strings.Split(tc.platform, "/")
		platform := v1.Platform{}
		if len(parts) > 0 {
			platform.OS = parts[0]
		}
		if len(parts) > 1 {
			platform.Architecture = parts[1]
		}
		if len(parts) > 2 {
			platform.Variant = parts[2]
		}
		if got := platformString(platform); got != tc.want {
			t.Errorf("platformString(%q) = %q, want %q", tc.platform, got, tc.want)
		}
	}
}
