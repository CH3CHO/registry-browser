// Package app 组装 Wails 服务，把内部能力暴露给前端。
//
// 这一层只做三件事：读取配置、调用 registry 客户端、把内部错误翻译成前端能显示的结构。
// 业务逻辑不在这里，UI 也不在这里。
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/ch3cho/registry-browser/internal/config"
	"github.com/ch3cho/registry-browser/internal/parse"
	"github.com/ch3cho/registry-browser/internal/registry"
)

// Service 是暴露给前端的 Wails 服务。
type Service struct {
	mu       sync.Mutex
	clients  map[string]*registry.Client
	settings *Settings
}

// NewService 创建服务并加载配置。
func NewService() (*Service, error) {
	settings, err := LoadSettings()
	if err != nil {
		return nil, err
	}
	return &Service{clients: map[string]*registry.Client{}, settings: settings}, nil
}

// RegistryInfo 是前端展示一个 registry 时需要的信息。
type RegistryInfo struct {
	// Name 是展示名，缺省时等于 Host。
	Name string `json:"name"`
	// Host 是 registry 主机名。
	Host string `json:"host"`
	// Namespace 是折叠用的命名空间。
	Namespace string `json:"namespace"`
	// BasePath 是厂商特有的路径前缀。
	BasePath string `json:"basePath"`
	// HasCredential 表示本地是否保存了该 registry 的凭据。
	HasCredential bool `json:"hasCredential"`
	// Source 说明该条目来自工具配置还是 Docker 配置。
	Source string `json:"source"`
}

// RepositoryInfo 是一个仓库在目录中的展示信息。
type RepositoryInfo struct {
	// Name 是完整的仓库路径。
	Name string `json:"name"`
	// Namespace 是仓库路径中除最后一段之外的部分。
	Namespace string `json:"namespace"`
	// Image 是仓库路径的最后一段。
	Image string `json:"image"`
}

// ImageEntry 是树中的一个镜像。
type ImageEntry struct {
	// Name 是镜像名（仓库路径的最后一段）。
	Name string `json:"name"`
	// Repository 是完整的仓库路径。
	Repository string `json:"repository"`
}

// NamespaceEntry 是树中的一个命名空间。
type NamespaceEntry struct {
	// Name 是命名空间名。
	Name string `json:"name"`
	// Images 是该命名空间下的镜像。
	Images []ImageEntry `json:"images"`
}

// TagDetail 是 tag 详情的完整视图：清单 + 镜像配置。
type TagDetail struct {
	// Manifest 是清单信息。
	Manifest *ManifestInfo `json:"manifest"`
	// Config 是镜像配置，包含 labels 与构建历史。
	Config *registry.ImageConfig `json:"config,omitempty"`
}

// TagInfo 是 tag 列表中的一项。
type TagInfo struct {
	// Name 是 tag 名。
	Name string `json:"name"`
}

// ManifestInfo 是前端渲染 tag 详情所需的数据。
type ManifestInfo struct {
	MediaType   string                      `json:"mediaType"`
	Digest      string                      `json:"digest"`
	Size        int64                       `json:"size"`
	Annotations map[string]string           `json:"annotations"`
	Config      *registry.Descriptor        `json:"config,omitempty"`
	Layers      []registry.Descriptor       `json:"layers"`
	Platforms   []registry.PlatformManifest `json:"platforms"`
	Raw         json.RawMessage             `json:"raw"`
}

// APIError 是返回给前端的错误结构。
type APIError struct {
	// Kind 是错误分类。
	Kind string `json:"kind"`
	// Status 是 HTTP 状态码，网络错误时为 0。
	Status int `json:"status"`
	// Message 是可展示的错误描述。
	Message string `json:"message"`
	// Hint 是给用户的处置建议。
	Hint string `json:"hint"`
}

// Error 让 APIError 可以作为 error 返回给前端。
func (e *APIError) Error() string {
	if e.Hint == "" {
		return e.Message
	}
	return e.Message + " — " + e.Hint
}

// ListRegistries 列出所有可用的 registry。
func (s *Service) ListRegistries() ([]RegistryInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	infos := make([]RegistryInfo, 0, len(s.settings.Registries))
	for _, entry := range s.settings.Registries {
		infos = append(infos, RegistryInfo{
			Name:          entry.DisplayName(),
			Host:          entry.Host,
			Namespace:     entry.Namespace,
			BasePath:      entry.BasePath,
			HasCredential: entry.HasCredential,
			Source:        entry.Source,
		})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Host < infos[j].Host })
	return infos, nil
}

// ParseReference 解析用户输入的镜像地址。
func (s *Service) ParseReference(input string) (parse.Reference, error) {
	return parse.Parse(input, parse.Options{KnownRegistries: s.knownRegistries()})
}

// ListRepositories 列出一个 registry 下的全部仓库。
func (s *Service) ListRepositories(host string) ([]RepositoryInfo, error) {
	client, err := s.client(host)
	if err != nil {
		return nil, err
	}

	repositories, err := client.ListRepositories(context.Background())
	if err != nil {
		return nil, toAPIError(err)
	}

	infos := make([]RepositoryInfo, 0, len(repositories))
	for _, repository := range repositories {
		namespace, image := splitRepository(repository)
		infos = append(infos, RepositoryInfo{
			Name:      repository,
			Namespace: namespace,
			Image:     image,
		})
	}
	return infos, nil
}

// ListNamespaces 列出一个 registry 下的命名空间与镜像，用于左侧目录树。
func (s *Service) ListNamespaces(host string) ([]NamespaceEntry, error) {
	client, err := s.client(host)
	if err != nil {
		return nil, err
	}

	repositories, err := client.ListRepositories(context.Background())
	if err != nil {
		return nil, toAPIError(err)
	}

	return groupRepositories(repositories, client.Namespace()), nil
}

// groupRepositories 按命名空间折叠仓库路径。配置了命名空间前缀时以它为准，
// 否则取仓库路径中除最后一段之外的部分。
func groupRepositories(repositories []string, namespace string) []NamespaceEntry {
	grouped := map[string][]ImageEntry{}

	for _, repository := range repositories {
		group, image := splitGroup(repository, namespace)
		grouped[group] = append(grouped[group], ImageEntry{
			Name:       image,
			Repository: repository,
		})
	}

	namespaces := make([]NamespaceEntry, 0, len(grouped))
	for name, images := range grouped {
		sort.Slice(images, func(i, j int) bool { return images[i].Name < images[j].Name })
		namespaces = append(namespaces, NamespaceEntry{Name: name, Images: images})
	}
	sort.Slice(namespaces, func(i, j int) bool { return namespaces[i].Name < namespaces[j].Name })

	return namespaces
}

// splitGroup 决定一个仓库归属的命名空间以及镜像名。
func splitGroup(repository, namespace string) (string, string) {
	parts := strings.Split(repository, "/")
	image := parts[len(parts)-1]

	if namespace != "" {
		if repository == namespace {
			return namespace, image
		}
		if strings.HasPrefix(repository, namespace+"/") {
			return namespace, image
		}
	}

	if len(parts) == 1 {
		return "", image
	}
	return strings.Join(parts[:len(parts)-1], "/"), image
}

// ListTags 列出一个仓库下的全部 tag。
func (s *Service) ListTags(host, repository string) ([]TagInfo, error) {
	client, err := s.client(host)
	if err != nil {
		return nil, err
	}

	tags, err := client.ListTags(context.Background(), repository)
	if err != nil {
		return nil, toAPIError(err)
	}

	infos := make([]TagInfo, 0, len(tags))
	for _, tag := range tags {
		infos = append(infos, TagInfo{Name: tag})
	}
	return infos, nil
}

// GetTagDetail 获取一个 tag 的完整详情：清单与镜像配置。
func (s *Service) GetTagDetail(host, repository, ref string) (*TagDetail, error) {
	client, err := s.client(host)
	if err != nil {
		return nil, err
	}

	manifest, err := client.GetManifest(context.Background(), repository, ref)
	if err != nil {
		return nil, toAPIError(err)
	}

	detail := &TagDetail{
		Manifest: &ManifestInfo{
			MediaType:   manifest.MediaType,
			Digest:      manifest.Digest,
			Size:        manifest.Size,
			Annotations: manifest.Annotations,
			Config:      manifest.Config,
			Layers:      manifest.Layers,
			Platforms:   manifest.Platforms,
			Raw:         manifest.Raw,
		},
	}

	// 镜像配置是额外一次请求，取不到时不影响清单展示。
	if imageConfig, err := client.GetImageConfig(context.Background(), repository, ref); err == nil {
		detail.Config = imageConfig
	}

	return detail, nil
}

// GetManifest 获取一个 tag 的清单详情。
func (s *Service) GetManifest(host, repository, ref string) (*ManifestInfo, error) {
	client, err := s.client(host)
	if err != nil {
		return nil, err
	}

	manifest, err := client.GetManifest(context.Background(), repository, ref)
	if err != nil {
		return nil, toAPIError(err)
	}

	return &ManifestInfo{
		MediaType:   manifest.MediaType,
		Digest:      manifest.Digest,
		Size:        manifest.Size,
		Annotations: manifest.Annotations,
		Config:      manifest.Config,
		Layers:      manifest.Layers,
		Platforms:   manifest.Platforms,
		Raw:         manifest.Raw,
	}, nil
}

// client 返回（或创建）一个 registry 客户端。
func (s *Service) client(host string) (*registry.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, ok := s.clients[host]; ok {
		return client, nil
	}

	entry, ok := s.settings.Find(host)
	if !ok {
		return nil, fmt.Errorf("unknown registry %q", host)
	}

	client, err := registry.New(registry.Config{
		Host:          entry.Host,
		BasePath:      entry.BasePath,
		Namespace:     entry.Namespace,
		Insecure:      entry.Insecure,
		SkipTLSVerify: entry.SkipTLSVerify,
		Credential:    entry.Credential,
	})
	if err != nil {
		return nil, err
	}

	s.clients[host] = client
	return client, nil
}

// knownRegistries 返回所有已知主机名，供地址解析器判定 registry。
func (s *Service) knownRegistries() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	known := make(map[string]struct{}, len(s.settings.Registries))
	for _, entry := range s.settings.Registries {
		known[entry.Host] = struct{}{}
	}
	return known
}

// splitRepository 把仓库路径拆成命名空间与镜像名。
func splitRepository(repository string) (string, string) {
	for i := len(repository) - 1; i >= 0; i-- {
		if repository[i] == '/' {
			return repository[:i], repository[i+1:]
		}
	}
	return "", repository
}

// toAPIError 把内部错误转换成前端可展示的结构。
func toAPIError(err error) error {
	classified := registry.AsError(err)
	return &APIError{
		Kind:    string(classified.Kind),
		Status:  classified.Status,
		Message: classified.Error(),
		Hint:    classified.UserHint(),
	}
}

// Settings 是工具自身的配置。
type Settings struct {
	// Registries 是所有可用的 registry 条目。
	Registries []RegistryEntry
}

// RegistryEntry 是一个 registry 的完整配置。
type RegistryEntry struct {
	// Name 是可选的展示名。
	Name string
	// Host 是 registry 主机名。
	Host string
	// Namespace 是折叠用的命名空间。
	Namespace string
	// BasePath 是厂商特有的路径前缀。
	BasePath string
	// Insecure 表示使用明文 http。
	Insecure bool
	// SkipTLSVerify 表示跳过证书校验。
	SkipTLSVerify bool
	// Credential 是来自 Docker 配置的凭据。
	Credential config.Credential
	// HasCredential 表示是否有可用凭据。
	HasCredential bool
	// Source 说明条目来源。
	Source string
}

// DisplayName 返回展示名，缺省时退回主机名。
func (e RegistryEntry) DisplayName() string {
	if e.Name != "" {
		return e.Name
	}
	return e.Host
}

// Find 按主机名查找条目。
func (s *Settings) Find(host string) (RegistryEntry, bool) {
	for _, entry := range s.Registries {
		if entry.Host == host {
			return entry, true
		}
	}
	return RegistryEntry{}, false
}

// LoadSettings 读取工具配置与 Docker 凭据，合并成 registry 列表。
func LoadSettings() (*Settings, error) {
	settings := &Settings{Registries: []RegistryEntry{}}

	toolConfig, err := loadToolConfig()
	if err != nil {
		return nil, err
	}

	dockerPath, err := config.DefaultDockerConfigPath()
	if err != nil {
		return nil, err
	}
	dockerConfig, err := config.LoadDockerConfig(dockerPath)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	for _, entry := range toolConfig {
		credential, hasCredential := dockerConfig.Credentials(entry.Host)
		settings.Registries = append(settings.Registries, RegistryEntry{
			Name:          entry.Name,
			Host:          entry.Host,
			Namespace:     entry.Namespace,
			BasePath:      entry.BasePath,
			Insecure:      entry.Insecure,
			SkipTLSVerify: entry.SkipTLSVerify,
			Credential:    credential,
			HasCredential: hasCredential,
			Source:        "tool",
		})
		seen[entry.Host] = struct{}{}
	}

	for _, host := range dockerConfig.Hosts() {
		if _, ok := seen[host]; ok {
			continue
		}
		credential, hasCredential := dockerConfig.Credentials(host)
		settings.Registries = append(settings.Registries, RegistryEntry{
			Host:          host,
			Credential:    credential,
			HasCredential: hasCredential,
			Source:        "docker",
		})
	}

	return settings, nil
}

// toolConfigFile 是工具配置文件的数据结构。
type toolConfigFile struct {
	Registries []toolConfigEntry `toml:"registry"`
}

// toolConfigEntry 是工具配置文件中单个 registry 的条目。
type toolConfigEntry struct {
	Name          string `toml:"name"`
	Host          string `toml:"host"`
	Namespace     string `toml:"namespace"`
	BasePath      string `toml:"base_path"`
	Insecure      bool   `toml:"insecure"`
	SkipTLSVerify bool   `toml:"skip_tls_verify"`
}

// loadToolConfig 读取工具自己的 registries.toml。
func loadToolConfig() ([]toolConfigEntry, error) {
	path, err := ToolConfigPath()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read tool config %q: %w", path, err)
	}

	var file toolConfigFile
	if err := toml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("failed to parse tool config %q: %w", path, err)
	}
	return file.Registries, nil
}

// ToolConfigPath 返回工具配置文件的路径。便携模式下与可执行文件同级。
func ToolConfigPath() (string, error) {
	if dir := os.Getenv("REGISTRY_BROWSER_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "registries.toml"), nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to locate executable: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), "data", "registries.toml"), nil
}

// DataDir 返回数据目录，首次使用时创建。
func DataDir() (string, error) {
	dir := os.Getenv("REGISTRY_BROWSER_DATA_DIR")
	if dir == "" {
		executable, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("failed to locate executable: %w", err)
		}
		dir = filepath.Join(filepath.Dir(executable), "data")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create data directory %q: %w", dir, err)
	}
	return dir, nil
}
