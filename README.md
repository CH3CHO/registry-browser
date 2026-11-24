# Registry Browser

浏览容器镜像仓库的桌面客户端：查看一个 registry 下有哪些仓库、每个仓库有哪些 tag，以及单个 tag 的完整清单信息。

**只读工具** —— 不提供推送、删除 tag、GC 等任何写操作。

面向 Docker Registry HTTP API V2 规范，因此 Docker Hub、Harbor、阿里云 ACR、腾讯云 TCR、
JFrog Artifactory 等实现了该规范的仓库都可以直接接入。

## 界面

三栏 master-detail：

| 区域 | 内容 |
|---|---|
| 顶部 | 镜像地址输入框（可整条粘贴 `registry/namespace/image:tag`），支持按 tag 直达 |
| 左栏 | registry 选择器 + 命名空间/镜像树（仅在 registry 支持 catalog 时可用） |
| 中栏 | tag 列表，支持名称过滤 |
| 右栏 | tag 详情：概要、多架构平台、镜像层、labels、环境变量、构建历史、原始清单 JSON、一键复制 pull 命令 |

## 技术栈

| 层 | 选型 | 说明 |
|---|---|---|
| 桌面壳 | Wails v3 (v3.1.0) | 单二进制，走系统 WebView2，不依赖 Electron |
| 后端 | Go 1.26 | `github.com/google/go-containerregistry` 负责 Distribution v2 协议 |
| 前端 | Vue 3 + TypeScript + Vite | 组件与设计令牌自建，不引入通用 UI 组件库 |

## 目录结构

```
main.go                 应用入口
internal/
  parse/                 镜像地址解析（纯函数，全边界单测覆盖）
  config/                读取 ~/.docker/config.json 凭据（只读、不落盘、不写日志）
  registry/              Distribution v2 客户端：目录分页、标签、清单、错误分类
  app/                   Wails 服务层：配置合并、调用客户端、错误翻译
frontend/                Vue 3 前端
build/                   Wails 构建配置（Windows / macOS / Linux）
```

## 关键行为约定

- **认证**：复用 `~/.docker/config.json` 里的凭据（`auth` 与 `identitytoken` 都支持）。
  凭据只在内存中使用，不写日志、不写缓存、不导出。
- **catalog 可选**：`/v2/_catalog` 是可选端点，缺失时降级为「手输镜像地址定点查询」。
  公共 registry（Docker Hub 等）通常以 400/404/405/501 回应，一律归类为「不支持」。
- **懒加载**：打开 tag 列表不会触发 N 个 manifest 请求；详情按需拉取。
- **错误分类**：401 / 403 / 404 / 405 / 429 / 5xx / 网络错误分别归类，UI 据此给出不同提示。
- **时间字段**：标准 Distribution v2 不提供「推送时间」，界面展示的是 image config 中的
  **构建时间**，并明确标注来源。

## 界面结构

三栏 master-detail：

| 区域 | 内容 |
|---|---|
| 顶部 | 镜像地址输入框（可整条粘贴 `registry/namespace/image:tag`），支持按 tag 直达 |
| 左栏 | registry 选择器 + 命名空间/镜像树（仅在 registry 支持 catalog 时可用） |
| 中栏 | tag 列表，支持名称过滤 |
| 右栏 | tag 详情：概要、多架构平台、镜像层、labels、环境变量、构建历史、原始清单 JSON、一键复制 pull 命令 |

设计取向为「瑞士 / 数据密集」：浅色为主、跟随系统切换深色，发丝级分隔线、等宽字体承载
digest 与 tag，所有颜色与间距集中在 `frontend/src/styles/tokens.css`。

## 本地开发

前置：Go 1.26+、Node 20+、`wails3` CLI。

```powershell
# 安装 Wails v3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0

# 生成前端绑定（改动 Go 服务结构后需要重新生成）
wails3 generate bindings

# 开发模式（带热重载）
wails3 task dev

# 构建
wails3 build

# 测试
go test -cover ./internal/...

# 真实 registry 冒烟测试（需要网络，默认不参与常规测试）
go test -tags integration -v -run TestLive ./internal/registry/
```

> Windows 上 `go test -race` 需要 cgo 与 C 编译器；当前环境 `CGO_ENABLED=0`，因此未启用。

## 配置

工具自身的配置位于可执行文件同级的 `data/registries.toml`（可用环境变量
`REGISTRY_BROWSER_DATA_DIR` 覆盖目录）：

```toml
[[registry]]
name = "Higress"                      # 可选，展示名
host = "higress-registry.cn-hangzhou.cr.aliyuncs.com"
namespace = "higress"                 # 折叠用的命名空间
base_path = ""                        # 厂商路径前缀，例如 /artifactory/api/docker/team
insecure = false                      # 使用明文 http
skip_tls_verify = false               # 跳过证书校验
```

未在工具配置中出现的 registry 会自动从 `~/.docker/config.json` 的 `auths` 中发现，
凭据同样从那里读取。

## 交付

- Windows：`wails3 build` 产出单个 exe（便携模式）。
- macOS：在 macOS 上构建，或使用 `wails3 build GOOS=darwin` 走 Docker 交叉编译
  （镜像体积约 800MB，构建缓存需放在 D 盘）。

### macOS 交叉编译脚本

```powershell
# Apple Silicon
pwsh -File .\scripts\build-macos.ps1 -Arch arm64

# 两个架构一起构建
pwsh -File .\scripts\build-macos.ps1 -Arch both

# 指定产物目录
pwsh -File .\scripts\build-macos.ps1 -Arch arm64 -OutputDir D:\release\registry-browser
```

脚本会检查 Go / `wails3` / Docker 守护进程，并**拒绝在 Docker 数据根位于 C 盘时继续**
（交叉编译镜像体积很大）。Rancher Desktop 需要把镜像存储迁到 D 盘：
Settings → Container Engine → Image store location。

> 交叉编译产物**未签名未公证**，要在 Mac 上分发还需执行 `codesign` 与 `notarytool`。

### 在 WSL 中跑 race detector

Windows 上的 race detector 需要 cgo 与 C 编译器。没有本地 C 工具链时，可以借用 WSL
（自带 gcc），注意把 Go 工具链与缓存都放在非系统盘：

```bash
export PATH=/mnt/d/Tools/go/bin:$PATH
export GOMODCACHE=/mnt/d/Temp/go/pkg/mod
export GOCACHE=/mnt/d/Temp/go/build
cd /mnt/d/path/to/registry-browser
go test -race ./internal/...
```

仓库里提供了现成脚本 `scripts/run-race-wsl.sh`。

## License

[MIT](LICENSE) © 2026 Kent Dong
