//go:build integration

package registry

import (
	"context"
	"testing"
	"time"
)

// TestLiveDockerHub 用公开的 Docker Hub 验证真实链路：令牌交换、tags 列举、
// 清单获取与镜像配置拉取。需要网络，默认不参与常规测试。
//
//	go test -tags integration ./internal/registry/ -run TestLive
func TestLiveDockerHub(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live registry test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := New(Config{Host: "docker.io", PageSize: 20})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tags, err := client.ListTags(ctx, "library/nginx")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) == 0 {
		t.Fatal("expected tags for library/nginx")
	}
	t.Logf("library/nginx 有 %d 个 tag", len(tags))

	manifest, err := client.GetManifest(ctx, "library/nginx", "1.27")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if manifest.Digest == "" {
		t.Error("expected a manifest digest")
	}
	t.Logf("manifest %s mediaType=%s layers=%d", manifest.Digest, manifest.MediaType, len(manifest.Layers))

	imageConfig, err := client.GetImageConfig(ctx, "library/nginx", "1.27")
	if err != nil {
		t.Fatalf("GetImageConfig: %v", err)
	}
	if imageConfig.OS == "" || imageConfig.Architecture == "" {
		t.Errorf("expected platform info, got %s/%s", imageConfig.OS, imageConfig.Architecture)
	}
	t.Logf("config os=%s arch=%s created=%s", imageConfig.OS, imageConfig.Architecture, imageConfig.Created)
}
