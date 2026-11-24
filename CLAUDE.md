# CLAUDE.md

本文件是本仓库给 AI 助手的工作说明。**改动代码前请先读完这份文件**，里面记录的是已经拍板的范围边界与环境坑，改之前先确认是否越界。

## 项目定位

`registry-browser` —— **只读**的容器镜像仓库浏览器（Go + Wails v3 + Vue 3）。
用于查看一个 registry 下有哪些仓库、每个仓库有哪些 tag，以及单个 tag 的完整清单信息。

不是 `docker` 命令行的替代品，不提供任何写操作。

## 范围边界（已拍板，不要自行扩大）

- **只读**：不提供推送、删除 tag、GC、镜像扫描、SBOM、签名验证。
- **不接厂商私有 API**（ACR OpenAPI / Harbor API / TCR API）。`/v2/_catalog` 不可用时降级为
  「手输镜像地址定点查询」，由用户手动输入。
- **不显示「推送时间」**：标准 Distribution v2 没有这个字段（`/v2/<name>/tags/list` 只返回
  name；manifest 里没有时间；image config 里的 `created` 是**构建时间**）。
  UI 必须标注来源，不能用错字段名。
- **不做全局搜索**：入口只有「选择已知 registry」和「输入完整镜像地址（含/不含 tag）」。
- **懒加载**：打开 tag 列表不得触发 N 个 manifest 请求；批量拉取摘要必须是用户显式动作。
- **凭据**：只读 `~/.docker/config.json`（`auth` 与 `identitytoken` 都支持），
  只在内存使用，**不落盘、不写日志、不导出**；失败时提示用户 `docker login`，
  不做浏览器 OIDC 登录流程。
- **多架构 tag 不显示聚合 size**（求和高估、取最大随意），改为逐平台列出。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3（v3.1.0），系统 WebView2，不依赖 Electron |
| 后端 | Go 1.26 + `github.com/google/go-containerregistry` v0.22.1 |
| 前端 | Vue 3 + TypeScript + Vite，组件与设计令牌自建 |

模块路径：`github.com/ch3cho/registry-browser`（个人开源项目）。

## 目录结构

```
main.go                 Wails 入口
internal/
  parse/                 镜像地址解析（纯函数，全边界单测覆盖）
  config/                读取 ~/.docker/config.json 凭据
  registry/              Distribution v2 客户端：目录分页、标签、清单、镜像配置、错误分类
  app/                   Wails 服务层：配置合并、调用客户端、错误翻译
frontend/src/
  styles/tokens.css      设计令牌（浅色默认 + 跟随系统深色）
  components/            RegistryTree / TagTable / DetailPanel / AddressBar
scripts/
  build-macos.ps1        从 Windows 交叉编译 macOS
  run-race-wsl.sh        在 WSL 里跑 race detector
```

## 常用命令

```powershell
# 生成前端绑定（改动 Go 服务结构后必须重跑）
wails3 generate bindings -ts

# 构建前端
cd frontend; npm run build

# 桌面构建
wails3 build

# 测试（覆盖率 83%~93%）
go test -cover ./internal/...

# 真实 registry 冒烟测试（需要网络，默认不跑）
go test -tags integration -v -run TestLive ./internal/registry/

# race detector（见下方环境说明）
wsl -d Ubuntu -u root -- bash /mnt/d/path/to/registry-browser/scripts/run-race-wsl.sh

# macOS 交叉编译（需要 Docker，且必须先构建 wails-cross 镜像）
wails3 task setup:docker
pwsh -File .\scripts\build-macos.ps1 -Arch arm64
```

## 环境坑（都是实测踩出来的）

- **C 盘禁令**：Go / npm / docker 的缓存一律落 D 盘。npm 缓存用
  `$env:npm_config_cache="<npm cache dir on a non-system drive>"`。
- **npm registry**：项目内 `frontend/.npmrc` 显式指向公共源 `https://registry.npmjs.org/`。
  需要内网 registry 时自行在该文件配置，**不要把凭据写进仓库**。
- **race detector**：本机 `CGO_ENABLED=0` 且无 gcc，Windows 上跑不了 `-race`；
  走 WSL Ubuntu（自带 gcc），Go 装在 `D:\Tools\go`，缓存也必须指向 D 盘。
- **含中文的 .ps1 必须用 `pwsh` 跑**，Windows PowerShell 5.1 会按 ANSI 解析直接语法报错。
- **`wails3 build` 没有 `-s` 参数**（v3.1.0），签名是独立的 `wails3 sign`。
- **macOS 交叉编译**必须先 `wails3 task setup:docker` 构建 `wails-cross` 镜像（约 5.5GB）；
  不做 macOS 构建时可以 `docker rmi wails-cross` 回收。
- **产物校验**：macOS 二进制的魔数应为 `CF FA ED FE`（Mach-O 64-bit），
  CPU type `0x0100000C` = arm64，`0x01000007` = x86_64。
- **zig 交叉编译会漏出 `-.o` 这类中间文件**，已用 `*.o` 忽略。

## 数据模型约定

- UI 语义：**registry → 命名空间 → 镜像 → tag**
- 物理语义（Distribution v2）：**repository → tag → manifest**
- 命名空间折叠规则：registry 配置了 `namespace` 前缀时以它为准，
  否则取仓库路径中除最后一段之外的部分（`team/sub/app` → 命名空间 `team/sub`、镜像 `app`）。
- 地址解析规则（`internal/parse`）：第一段含 `.` 或 `:` 视为 registry 主机，
  否则视为命名空间；Docker Hub 单段名称补全为 `library/<name>`；
  tag 是最后一个 `/` 之后的最后一个 `:` 之后的内容；`@` 之后是 digest。

## 提交约定

- 提交信息用英文，遵循 conventional commits。
- 推送远端需要用户明确确认，不要自行 push。
