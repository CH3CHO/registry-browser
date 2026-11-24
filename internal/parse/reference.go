// Package parse 解析容器镜像引用（image reference）。
//
// 解析规则见共识文档 Q18：
//   - 第一个路径段含 '.' 或 ':' 时视为 registry 主机，否则视为命名空间；
//   - 已知 registry（Options.KnownRegistries）可覆盖该判定；
//   - docker.io 下的单段名称补全为 library/<name>；
//   - namespace 取最后一个路径段之外的全部部分，image 取最后一段；
//   - tag 为最后一个 '/' 之后的最后一个 ':' 之后的内容；'@' 之后是 digest。
package parse

import (
	"errors"
	"fmt"
	"strings"
)

// DefaultRegistry 是未显式指定主机时使用的默认 registry。
const DefaultRegistry = "docker.io"

// defaultNamespace 是 Docker Hub 官方镜像的命名空间。
const defaultNamespace = "library"

var (
	// ErrEmptyInput 表示输入为空。
	ErrEmptyInput = errors.New("image reference is empty")
	// ErrInvalidRepository 表示仓库路径不合法。
	ErrInvalidRepository = errors.New("invalid repository name")
	// ErrInvalidTag 表示 tag 不合法。
	ErrInvalidTag = errors.New("invalid tag")
	// ErrInvalidDigest 表示 digest 不合法。
	ErrInvalidDigest = errors.New("invalid digest")
	// ErrInvalidRegistry 表示 registry 主机名不合法。
	ErrInvalidRegistry = errors.New("invalid registry host")
)

// Reference 是一次成功解析后的镜像引用。
type Reference struct {
	// Registry 是主机名，可含端口，例如 "higress-registry.cn-hangzhou.cr.aliyuncs.com"。
	Registry string `json:"registry"`
	// Repository 是去掉 registry 之后的完整仓库路径，例如 "higress/all-in-one"。
	Repository string `json:"repository"`
	// Namespace 是仓库路径中除最后一段之外的部分，例如 "higress"。
	Namespace string `json:"namespace"`
	// Image 是仓库路径的最后一段，例如 "all-in-one"。
	Image string `json:"image"`
	// Tag 可为空。
	Tag string `json:"tag"`
	// Digest 可为空，形如 "sha256:<hex>"。
	Digest string `json:"digest"`
}

// Options 控制解析行为。
type Options struct {
	// KnownRegistries 是被显式登记为 registry 主机的主机名集合。
	// 命中其中的主机即使不含 '.' 或 ':' 也会被当作 registry 而非命名空间。
	KnownRegistries map[string]struct{}
}

// Parse 解析镜像引用。输入不会被修改。
func Parse(input string, opts Options) (Reference, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Reference{}, ErrEmptyInput
	}

	name, digest, err := splitDigest(trimmed)
	if err != nil {
		return Reference{}, err
	}

	name, tag, err := splitTag(name)
	if err != nil {
		return Reference{}, err
	}

	segments := strings.Split(name, "/")
	for _, segment := range segments {
		if segment == "" {
			return Reference{}, fmt.Errorf("%w: empty path segment in %q", ErrInvalidRepository, name)
		}
	}

	registry := ""
	rest := segments
	if isRegistryHost(segments[0], opts) {
		registry = segments[0]
		rest = segments[1:]
	}
	if len(rest) == 0 {
		return Reference{}, fmt.Errorf("%w: missing repository path in %q", ErrInvalidRepository, name)
	}

	if registry == "" {
		registry = DefaultRegistry
		// Docker Hub 的单段名称是官方镜像，补全为 library/<name>。
		if len(rest) == 1 {
			rest = append([]string{defaultNamespace}, rest...)
		}
	}

	if err := validateRegistry(registry); err != nil {
		return Reference{}, err
	}
	for _, segment := range rest {
		if err := validatePathSegment(segment); err != nil {
			return Reference{}, fmt.Errorf("%w: %q", err, segment)
		}
	}

	return Reference{
		Registry:   registry,
		Repository: strings.Join(rest, "/"),
		Namespace:  strings.Join(rest[:len(rest)-1], "/"),
		Image:      rest[len(rest)-1],
		Tag:        tag,
		Digest:     digest,
	}, nil
}

// splitDigest 拆出 '@' 之后的 digest 部分。
func splitDigest(input string) (string, string, error) {
	at := strings.Index(input, "@")
	if at < 0 {
		return input, "", nil
	}

	name := input[:at]
	digest := input[at+1:]
	if name == "" {
		return "", "", fmt.Errorf("%w: missing name before digest", ErrInvalidRepository)
	}
	if err := validateDigest(digest); err != nil {
		return "", "", err
	}
	return name, digest, nil
}

// splitTag 拆出 tag。只有位于最后一个 '/' 之后的 ':' 才是 tag 分隔符。
func splitTag(name string) (string, string, error) {
	slash := strings.LastIndex(name, "/")
	colon := strings.LastIndex(name, ":")
	if colon <= slash {
		return name, "", nil
	}

	tag := name[colon+1:]
	base := name[:colon]
	if err := validateTag(tag); err != nil {
		return "", "", err
	}
	return base, tag, nil
}

// registryHostOf 返回输入中作为 registry 主机的第一段；不是主机时返回空字符串。
func registryHostOf(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}

	first := trimmed
	if slash := strings.Index(first, "/"); slash >= 0 {
		first = first[:slash]
	}
	if strings.Contains(first, ".") || strings.Contains(first, ":") {
		return first
	}
	return ""
}

// isRegistryHost 判断路径首段是否为 registry 主机。
func isRegistryHost(segment string, opts Options) bool {
	if strings.Contains(segment, ".") || strings.Contains(segment, ":") {
		return true
	}
	_, ok := opts.KnownRegistries[segment]
	return ok
}

// validateRegistry 校验 registry 主机名。
func validateRegistry(registry string) error {
	if registry == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidRegistry)
	}
	if len(registry) > 255 {
		return fmt.Errorf("%w: %q is too long", ErrInvalidRegistry, registry)
	}
	for _, r := range registry {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_' || r == ':':
		default:
			return fmt.Errorf("%w: %q contains invalid character %q", ErrInvalidRegistry, registry, r)
		}
	}
	return nil
}

// validatePathSegment 校验仓库路径的一段：小写字母、数字，以及 . _ - 分隔符。
// 形式为 alnum (sep alnum)*，其中 sep 可以是 '.'、'_' 或一个或多个 '-'。
func validatePathSegment(segment string) error {
	if segment == "" {
		return fmt.Errorf("%w: empty segment", ErrInvalidRepository)
	}

	expectAlnum := true
	afterDash := false
	for _, r := range segment {
		if expectAlnum {
			if !isLowerAlnum(r) {
				return fmt.Errorf("%w: %q contains invalid character %q", ErrInvalidRepository, segment, r)
			}
			expectAlnum = false
			afterDash = false
			continue
		}

		switch r {
		case '-':
			// 连续的 '-' 是合法的，'.' 与 '_' 不能跟在 '-' 之后。
			if afterDash {
				continue
			}
			afterDash = true
			expectAlnum = true
		case '.':
			if afterDash {
				return fmt.Errorf("%w: %q has a misplaced separator", ErrInvalidRepository, segment)
			}
			afterDash = false
			expectAlnum = true
		case '_':
			if afterDash {
				return fmt.Errorf("%w: %q has a misplaced separator", ErrInvalidRepository, segment)
			}
			afterDash = false
			expectAlnum = true
		default:
			if !isLowerAlnum(r) {
				return fmt.Errorf("%w: %q contains invalid character %q", ErrInvalidRepository, segment, r)
			}
			expectAlnum = false
			afterDash = false
		}
	}

	if expectAlnum {
		return fmt.Errorf("%w: %q must end with an alphanumeric character", ErrInvalidRepository, segment)
	}
	return nil
}

// isLowerAlnum 判断字符是否为小写字母或数字。
func isLowerAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

// validateTag 校验 tag：长度不超过 128，首字符为字母数字或下划线。
func validateTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("%w: empty tag", ErrInvalidTag)
	}
	if len(tag) > 128 {
		return fmt.Errorf("%w: %q exceeds 128 characters", ErrInvalidTag, tag)
	}
	for i, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_':
			if i == 0 {
				return fmt.Errorf("%w: %q must not start with an underscore", ErrInvalidTag, tag)
			}
		case r == '.' || r == '-':
		default:
			return fmt.Errorf("%w: %q contains invalid character %q", ErrInvalidTag, tag, r)
		}
	}
	return nil
}

// digestHexLengths 是受支持的摘要算法及其十六进制长度。
var digestHexLengths = map[string]int{
	"sha256": 64,
	"sha512": 128,
}

// validateDigest 校验 "algorithm:hex" 形式的 digest。
func validateDigest(digest string) error {
	algorithm, hex, found := strings.Cut(digest, ":")
	if !found {
		return fmt.Errorf("%w: %q is missing an algorithm separator", ErrInvalidDigest, digest)
	}

	expected, ok := digestHexLengths[algorithm]
	if !ok {
		return fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidDigest, algorithm)
	}
	if len(hex) != expected {
		return fmt.Errorf("%w: %q should have %d hex characters, got %d", ErrInvalidDigest, digest, expected, len(hex))
	}
	for _, r := range hex {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return fmt.Errorf("%w: %q contains invalid hex character %q", ErrInvalidDigest, digest, r)
		}
	}
	return nil
}
