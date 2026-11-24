// Package config 负责读取本工具的配置与 Docker 客户端凭据。
//
// 安全约定（共识 Q3 / Q11 / Q17）：
//   - 凭据只在内存中使用，不写缓存、不写日志、不导出到文件；
//   - 读取 ~/.docker/config.json 是唯一读取用户凭据的途径；
//   - 工具自身配置（registries.toml）永远不保存密码。
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credential 是一个 registry 的凭据。三种形态互斥优先级为：
// identitytoken > auth > username/password。
type Credential struct {
	// Username 与 Password 来自 base64 编码的 "user:pass" 或明文字段。
	Username string
	Password string
	// IdentityToken 是 OIDC 登录态下保存的令牌，可直接作为 Bearer token 使用。
	IdentityToken string
}

// HasSecret 表示该凭据是否包含可用于认证的秘密。
func (c Credential) HasSecret() bool {
	return c.Username != "" || c.Password != "" || c.IdentityToken != ""
}

// DockerConfig 是 ~/.docker/config.json 的解析结果。
type DockerConfig struct {
	// Auths 以 registry 主机名为键，保存凭据。
	Auths map[string]Credential
	// CredsStore 是全局凭据助手的名称。
	CredsStore string
	// CredHelpers 是按 registry 指定的凭据助手。
	CredHelpers map[string]string
}

// dockerConfigFile 镜像 ~/.docker/config.json 的结构。
type dockerConfigFile struct {
	Auths       map[string]dockerAuthEntry `json:"auths"`
	CredsStore  string                     `json:"credsStore"`
	CredHelpers map[string]string          `json:"credHelpers"`
}

// dockerAuthEntry 镜像配置文件中单个 registry 的条目。
type dockerAuthEntry struct {
	Auth          string `json:"auth"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	IdentityToken string `json:"identitytoken"`
}

// LoadDockerConfig 读取 Docker 客户端配置文件。文件不存在时返回空配置而非错误。
func LoadDockerConfig(path string) (*DockerConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &DockerConfig{Auths: map[string]Credential{}}, nil
		}
		return nil, fmt.Errorf("failed to read docker config %q: %w", path, err)
	}

	var file dockerConfigFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("failed to parse docker config %q: %w", path, err)
	}

	cfg := &DockerConfig{
		Auths:       make(map[string]Credential, len(file.Auths)),
		CredsStore:  file.CredsStore,
		CredHelpers: file.CredHelpers,
	}
	if cfg.CredHelpers == nil {
		cfg.CredHelpers = map[string]string{}
	}

	for host, entry := range file.Auths {
		cred, err := credentialFromEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to parse credentials for %q in %q: %w", host, path, err)
		}
		for _, key := range credentialKeys(host) {
			cfg.Auths[key] = cred
		}
	}
	return cfg, nil
}

// credentialFromEntry 把配置条目转换成凭据。
func credentialFromEntry(entry dockerAuthEntry) (Credential, error) {
	if entry.IdentityToken != "" {
		return Credential{IdentityToken: entry.IdentityToken}, nil
	}

	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return Credential{}, fmt.Errorf("invalid base64 auth: %w", err)
		}
		username, password, found := strings.Cut(string(decoded), ":")
		if !found {
			// 与 docker 客户端保持一致：没有冒号时整段都当作密码。
			return Credential{Password: string(decoded)}, nil
		}
		return Credential{Username: username, Password: password}, nil
	}

	return Credential{Username: entry.Username, Password: entry.Password}, nil
}

// credentialKeys 返回一个主机名对应的所有等价键，用于按不同写法查找凭据。
func credentialKeys(host string) []string {
	keys := []string{host}
	switch host {
	case "https://index.docker.io/v1/", "index.docker.io", "registry-1.docker.io":
		keys = append(keys, "docker.io")
	case "docker.io":
		keys = append(keys, "index.docker.io", "registry-1.docker.io")
	}
	return keys
}

// Credentials 按主机名查找凭据，查找时会尝试等价写法。
func (c *DockerConfig) Credentials(host string) (Credential, bool) {
	for _, key := range credentialKeys(host) {
		if cred, ok := c.Auths[key]; ok && cred.HasSecret() {
			return cred, true
		}
	}
	return Credential{}, false
}

// Hosts 返回配置中出现过的全部 registry 主机名（去重，已归一化）。
func (c *DockerConfig) Hosts() []string {
	seen := map[string]struct{}{}
	hosts := make([]string, 0, len(c.Auths))
	for host := range c.Auths {
		key := normalizeHost(host)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		hosts = append(hosts, key)
	}
	return hosts
}

// normalizeHost 把 Docker Hub 的历史写法归一化为 docker.io。
func normalizeHost(host string) string {
	switch host {
	case "https://index.docker.io/v1/", "index.docker.io", "registry-1.docker.io":
		return "docker.io"
	default:
		return host
	}
}

// DefaultDockerConfigPath 返回默认的 Docker 配置文件路径。
func DefaultDockerConfigPath() (string, error) {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	return filepath.Join(home, ".docker", "config.json"), nil
}
