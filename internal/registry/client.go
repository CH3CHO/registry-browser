// Package registry 提供对 Docker Registry HTTP API V2 的只读访问。
//
// 设计要点（来自项目共识）：
//   - 只读：不提供任何写操作；
//   - 可选端点要能优雅降级：catalog 不可用时由上层降级为手输镜像名；
//   - 厂商差异收敛在 Config：base path（Artifactory）、命名空间、TLS 开关；
//   - 错误必须分类，UI 才能区分「没登录 / 没权限 / 不存在 / 不支持」。
package registry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/ch3cho/registry-browser/internal/config"
)

// 默认的分页大小与超时。
const (
	defaultPageSize = 100
	defaultTimeout  = 30 * time.Second
)

// Config 描述一个 registry 的连接参数。
type Config struct {
	// Host 是 registry 主机名，可含端口。
	Host string
	// BasePath 是可选的路径前缀，例如 Artifactory 的 /artifactory/api/docker/<repoKey>。
	BasePath string
	// Namespace 是折叠用的命名空间提示。
	Namespace string
	// Insecure 表示使用明文 http。
	Insecure bool
	// SkipTLSVerify 表示跳过证书校验。
	SkipTLSVerify bool
	// Credential 是该 registry 的凭据，只在内存中使用。
	Credential config.Credential
	// PageSize 是 catalog 与 tags 的分页大小。
	PageSize int
}

// Descriptor 描述一个内容寻址对象。
type Descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// PlatformManifest 描述多架构清单中的一个平台。
type PlatformManifest struct {
	// Platform 形如 "linux/amd64"。
	Platform string `json:"platform"`
	Descriptor
}

// Manifest 是一个 tag 对应的清单信息。
type Manifest struct {
	// MediaType 是清单的媒体类型。
	MediaType string
	// Digest 是清单的摘要。
	Digest string
	// Size 是清单字节数。
	Size int64
	// Annotations 是清单上的注解。
	Annotations map[string]string
	// Config 是镜像配置描述符。
	Config *Descriptor
	// Layers 是镜像层描述符。
	Layers []Descriptor
	// Platforms 在多架构清单下非空。
	Platforms []PlatformManifest
	// Raw 是原始清单 JSON。
	Raw []byte
}

// Client 是一个 registry 的只读客户端。
type Client struct {
	cfg      Config
	registry name.Registry
	options  []remote.Option
}

// New 创建一个 registry 客户端。
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("registry host is required")
	}
	if cfg.PageSize <= 0 {
		cfg.PageSize = defaultPageSize
	}

	nameOptions := []name.Option{name.WeakValidation}
	if cfg.Insecure {
		nameOptions = append(nameOptions, name.Insecure)
	}

	reg, err := name.NewRegistry(cfg.Host, nameOptions...)
	if err != nil {
		return nil, fmt.Errorf("invalid registry host %q: %w", cfg.Host, err)
	}

	roundTripper, err := buildTransport(cfg)
	if err != nil {
		return nil, err
	}

	options := []remote.Option{
		remote.WithTransport(roundTripper),
		remote.WithAuth(authenticatorFor(cfg.Credential)),
		remote.WithPageSize(cfg.PageSize),
	}

	return &Client{cfg: cfg, registry: reg, options: options}, nil
}

// Host 返回 registry 主机名。
func (c *Client) Host() string { return c.cfg.Host }

// Namespace 返回配置的命名空间。
func (c *Client) Namespace() string { return c.cfg.Namespace }

// ListRepositories 列出 registry 中的全部仓库，按需分页。
func (c *Client) ListRepositories(ctx context.Context) ([]string, error) {
	var (
		repositories []string
		last         string
	)

	for {
		page, err := remote.CatalogPage(c.registry, last, c.cfg.PageSize, c.withContext(ctx)...)
		if err != nil {
			return nil, c.wrapCatalog(err)
		}
		if len(page) == 0 {
			break
		}

		repositories = append(repositories, page...)

		next := page[len(page)-1]
		if next == last {
			// registry 没有推进游标，继续请求只会重复。
			break
		}
		last = next

		if len(page) < c.cfg.PageSize {
			break
		}
	}

	return repositories, nil
}

// ListTags 列出仓库下的全部 tag。
func (c *Client) ListTags(ctx context.Context, repository string) ([]string, error) {
	repo, err := c.repository(repository)
	if err != nil {
		return nil, c.wrap("list tags", err)
	}

	tags, err := remote.List(repo, c.withContext(ctx)...)
	if err != nil {
		return nil, c.wrap("list tags", err)
	}
	return tags, nil
}

// GetManifest 获取 tag 或 digest 对应的清单。
func (c *Client) GetManifest(ctx context.Context, repository, ref string) (*Manifest, error) {
	descriptor, err := c.fetchDescriptor(ctx, repository, ref)
	if err != nil {
		return nil, c.wrap("get manifest", err)
	}

	return parseManifest(descriptor)
}

// fetchDescriptor 按 tag 或 digest 取回清单描述符。
func (c *Client) fetchDescriptor(ctx context.Context, repository, ref string) (*remote.Descriptor, error) {
	repo, err := c.repository(repository)
	if err != nil {
		return nil, err
	}

	options := c.withContext(ctx)
	if strings.Contains(ref, ":") {
		return remote.Get(repo.Digest(ref), options...)
	}
	return remote.Get(repo.Tag(ref), options...)
}

// withContext 在保留默认选项的前提下附加调用方的上下文，使超时与取消可以生效。
func (c *Client) withContext(ctx context.Context) []remote.Option {
	options := make([]remote.Option, 0, len(c.options)+1)
	options = append(options, c.options...)
	if ctx != nil {
		options = append(options, remote.WithContext(ctx))
	}
	return options
}

// repository 把仓库路径解析为带 registry 主机的引用。
func (c *Client) repository(path string) (name.Repository, error) {
	nameOptions := []name.Option{name.WeakValidation}
	if c.cfg.Insecure {
		nameOptions = append(nameOptions, name.Insecure)
	}

	full := path
	if !strings.Contains(path, "/") || !strings.HasPrefix(path, c.cfg.Host) {
		full = c.cfg.Host + "/" + path
	}

	return name.NewRepository(full, nameOptions...)
}

// wrap 把底层错误包装成带分类的错误。
func (c *Client) wrap(op string, err error) error {
	classified := Classify(err)
	if classified == nil {
		return nil
	}
	return &Error{
		Kind:   classified.Kind,
		Status: classified.Status,
		Op:     op,
		Err:    classified.Err,
	}
}

// wrapCatalog 包装列目录的错误。多数公共 registry 并不实现 /v2/_catalog，
// 会以 400/404/405/501 回应，这些都归为「不支持」而不是「出错了」。
func (c *Client) wrapCatalog(err error) error {
	classified := Classify(err)
	if classified == nil {
		return nil
	}

	switch classified.Status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		classified.Kind = KindUnsupported
	}

	return &Error{
		Kind:   classified.Kind,
		Status: classified.Status,
		Op:     "list repositories",
		Err:    classified.Err,
	}
}

// buildTransport 构造 HTTP 传输层，按需注入 base path 与 TLS 策略。
func buildTransport(cfg Config) (http.RoundTripper, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("unexpected default transport type")
	}
	transport := base.Clone()
	transport.ResponseHeaderTimeout = defaultTimeout

	if cfg.SkipTLSVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	if cfg.BasePath != "" {
		return &basePathTransport{
			basePath: strings.TrimSuffix(cfg.BasePath, "/"),
			next:     transport,
		}, nil
	}
	return transport, nil
}

// basePathTransport 为 /v2 路径加上 registry 特有的前缀。
type basePathTransport struct {
	basePath string
	next     http.RoundTripper
}

// RoundTrip 实现 http.RoundTripper。
func (t *basePathTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/v2" || strings.HasPrefix(req.URL.Path, "/v2/") {
		clone := req.Clone(req.Context())
		clone.URL.Path = t.basePath + req.URL.Path
		return t.next.RoundTrip(clone)
	}
	return t.next.RoundTrip(req)
}

// authenticatorFor 把凭据转换成认证器。identity token 走库的 OAuth 流程，与 docker 客户端一致。
func authenticatorFor(cred config.Credential) authn.Authenticator {
	if !cred.HasSecret() {
		return authn.Anonymous
	}
	return authn.FromConfig(authn.AuthConfig{
		Username:      cred.Username,
		Password:      cred.Password,
		IdentityToken: cred.IdentityToken,
	})
}

// parseManifest 把 go-containerregistry 的描述符转换成领域模型。
func parseManifest(descriptor *remote.Descriptor) (*Manifest, error) {
	manifest := &Manifest{
		MediaType: string(descriptor.MediaType),
		Digest:    descriptor.Digest.String(),
		Size:      descriptor.Size,
		Raw:       descriptor.Manifest,
	}

	var envelope struct {
		Annotations map[string]string `json:"annotations"`
		Config      *Descriptor       `json:"config"`
		Layers      []Descriptor      `json:"layers"`
		Manifests   []struct {
			Descriptor
			Platform *struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
				Variant      string `json:"variant"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(descriptor.Manifest, &envelope); err != nil {
		return nil, &Error{Kind: KindInvalidResponse, Op: "get manifest", Err: err}
	}

	manifest.Annotations = envelope.Annotations
	manifest.Config = envelope.Config
	manifest.Layers = envelope.Layers

	for _, child := range envelope.Manifests {
		platform := ""
		if child.Platform != nil {
			platform = child.Platform.OS + "/" + child.Platform.Architecture
			if child.Platform.Variant != "" {
				platform += "/" + child.Platform.Variant
			}
		}
		manifest.Platforms = append(manifest.Platforms, PlatformManifest{
			Platform:   platform,
			Descriptor: child.Descriptor,
		})
	}

	return manifest, nil
}

// ensure transport package is referenced for future error helpers.
var _ = transport.Error{}
