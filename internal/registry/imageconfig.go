package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// ImageConfig 是镜像配置（image config blob）的可展示视图。
type ImageConfig struct {
	// Platform 是配置所属的平台，例如 "linux/amd64"。
	Platform string `json:"platform,omitempty"`
	// Architecture 是 CPU 架构。
	Architecture string `json:"architecture,omitempty"`
	// OS 是操作系统。
	OS string `json:"os,omitempty"`
	// Variant 是架构变体，例如 "v8"。
	Variant string `json:"variant,omitempty"`
	// Created 是镜像构建时间。注意：这不是推送时间。
	Created string `json:"created,omitempty"`
	// Author 是镜像作者。
	Author string `json:"author,omitempty"`
	// DockerVersion 是构建工具版本。
	DockerVersion string `json:"dockerVersion,omitempty"`
	// Labels 是镜像标签。
	Labels map[string]string `json:"labels,omitempty"`
	// Env 是环境变量。
	Env []string `json:"env,omitempty"`
	// Entrypoint 是入口命令。
	Entrypoint []string `json:"entrypoint,omitempty"`
	// Cmd 是默认命令。
	Cmd []string `json:"cmd,omitempty"`
	// WorkingDir 是工作目录。
	WorkingDir string `json:"workingDir,omitempty"`
	// User 是默认用户。
	User string `json:"user,omitempty"`
	// ExposedPorts 是暴露的端口。
	ExposedPorts map[string]struct{} `json:"exposedPorts,omitempty"`
	// Volumes 是声明的卷。
	Volumes map[string]struct{} `json:"volumes,omitempty"`
	// History 是构建历史。
	History []HistoryEntry `json:"history,omitempty"`
}

// HistoryEntry 是一条构建历史。
type HistoryEntry struct {
	// Created 是该步骤的时间。
	Created string `json:"created,omitempty"`
	// CreatedBy 是该步骤执行的指令。
	CreatedBy string `json:"createdBy,omitempty"`
	// Comment 是注释。
	Comment string `json:"comment,omitempty"`
	// EmptyLayer 表示该步骤没有产生镜像层。
	EmptyLayer bool `json:"emptyLayer,omitempty"`
}

// GetImageConfig 获取镜像配置。多架构镜像会按平台选择一个子清单。
func (c *Client) GetImageConfig(ctx context.Context, repository, ref string) (*ImageConfig, error) {
	desc, err := c.fetchDescriptor(ctx, repository, ref)
	if err != nil {
		return nil, c.wrap("get image config", err)
	}

	image, platform, err := resolveImage(desc)
	if err != nil {
		return nil, c.wrap("get image config", err)
	}

	configFile, err := image.ConfigFile()
	if err != nil {
		return nil, c.wrap("get image config", err)
	}

	return convertConfig(configFile, platform), nil
}

// resolveImage 从清单描述符解析出可读的镜像。多架构清单会挑选一个平台。
func resolveImage(desc *remote.Descriptor) (v1.Image, string, error) {
	if isIndex(desc.MediaType) {
		index, err := desc.ImageIndex()
		if err != nil {
			return nil, "", fmt.Errorf("failed to read image index: %w", err)
		}
		hash, platform := pickPlatform(index)
		image, err := index.Image(hash)
		if err != nil {
			return nil, "", fmt.Errorf("failed to select platform %s: %w", platformString(platform), err)
		}
		return image, platformString(platform), nil
	}

	image, err := desc.Image()
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image: %w", err)
	}
	return image, "", nil
}

// isIndex 判断媒体类型是否是多架构清单。
func isIndex(mediaType types.MediaType) bool {
	return mediaType == types.OCIImageIndex || mediaType == types.DockerManifestList
}

// pickPlatform 从多架构清单中挑选一个平台，优先 linux/amd64，并返回对应的摘要。
func pickPlatform(index v1.ImageIndex) (v1.Hash, v1.Platform) {
	manifest, err := index.IndexManifest()
	if err != nil {
		return v1.Hash{}, v1.Platform{}
	}

	var (
		fallbackHash     v1.Hash
		fallbackPlatform v1.Platform
	)
	for _, descriptor := range manifest.Manifests {
		if descriptor.Platform == nil {
			continue
		}
		if descriptor.Platform.OS == "linux" && descriptor.Platform.Architecture == "amd64" {
			return descriptor.Digest, *descriptor.Platform
		}
		if fallbackPlatform.OS == "" {
			fallbackHash, fallbackPlatform = descriptor.Digest, *descriptor.Platform
		}
	}
	return fallbackHash, fallbackPlatform
}

// platformString 把平台格式化成 "os/arch[/variant]"。
func platformString(platform v1.Platform) string {
	if platform.OS == "" && platform.Architecture == "" {
		return ""
	}
	value := platform.OS + "/" + platform.Architecture
	if platform.Variant != "" {
		value += "/" + platform.Variant
	}
	return value
}

// convertConfig 把库里的配置结构转换成领域模型。
func convertConfig(configFile *v1.ConfigFile, platform string) *ImageConfig {
	if configFile == nil {
		return nil
	}

	converted := &ImageConfig{
		Platform:      platform,
		Architecture:  configFile.Architecture,
		OS:            configFile.OS,
		Variant:       configFile.Variant,
		Created:       configFile.Created.Time.UTC().Format("2006-01-02T15:04:05Z"),
		Author:        configFile.Author,
		DockerVersion: configFile.DockerVersion,
		Labels:        configFile.Config.Labels,
		Env:           configFile.Config.Env,
		Entrypoint:    configFile.Config.Entrypoint,
		Cmd:           configFile.Config.Cmd,
		WorkingDir:    configFile.Config.WorkingDir,
		User:          configFile.Config.User,
		ExposedPorts:  configFile.Config.ExposedPorts,
		Volumes:       configFile.Config.Volumes,
	}

	for _, entry := range configFile.History {
		converted.History = append(converted.History, HistoryEntry{
			Created:    entry.Created.Time.UTC().Format("2006-01-02T15:04:05Z"),
			CreatedBy:  strings.TrimSpace(entry.CreatedBy),
			Comment:    entry.Comment,
			EmptyLayer: entry.EmptyLayer,
		})
	}

	return converted
}
