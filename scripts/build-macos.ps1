<#
.SYNOPSIS
    构建 macOS 版本的 Registry Browser（从 Windows 交叉编译）。

.DESCRIPTION
    Wails v3 提供 Docker 交叉编译链路：在非 macOS 主机上，用 build/docker/Dockerfile.cross
    构建 wails-cross 镜像（Zig + macOS SDK + Go 工具链），再在容器里完成 darwin 目标构建。

    本脚本负责：
      1. 前置检查（Go / wails3 / Docker 守护进程）；
      2. 校验 Docker 数据根不在 C 盘 —— 交叉编译镜像体积很大，落到 C 盘会挤占系统盘；
      3. 调用 wails3 完成交叉编译；
      4. 把产物收集到 dist/macos/<arch>/。

    注意：交叉编译产物**未签名、未公证**，要在 Mac 上分发还需要 codesign + notarytool。

.EXAMPLE
    .\scripts\build-macos.ps1 -Arch arm64

.EXAMPLE
    .\scripts\build-macos.ps1 -Arch both -OutputDir D:\release\registry-browser
#>
[CmdletBinding()]
param(
    # 目标架构。arm64 覆盖 Apple Silicon，amd64 覆盖 Intel Mac。
    [ValidateSet('arm64', 'amd64', 'both')]
    [string] $Arch = 'arm64',

    # 产物输出目录（相对于仓库根目录，也可以给绝对路径）。
    [string] $OutputDir = 'dist/macos',

    # 允许 Docker 数据根位于 C 盘。仅在磁盘实在放不下时使用。
    [switch] $AllowDockerOnC,

    # 跳过所有前置检查（不建议）。
    [switch] $SkipPreflight
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $RepoRoot

function Write-Step {
    param([string] $Message)
    Write-Host ''
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Ok {
    param([string] $Message)
    Write-Host "    $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string] $Message)
    Write-Host "    $Message" -ForegroundColor Yellow
}

function Fail {
    param([string] $Message)
    Write-Host "    $Message" -ForegroundColor Red
    Pop-Location
    exit 1
}

try {
    # ---------------------------------------------------------------- 前置检查
    if (-not $SkipPreflight) {
        Write-Step '检查前置依赖'

        if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
            Fail '找不到 go，请先安装 Go 1.26+ 并加入 PATH。'
        }
        $goVersion = (go version)
        Write-Ok $goVersion

        if (-not (Get-Command wails3 -ErrorAction SilentlyContinue)) {
            Write-Warn '找不到 wails3，正在安装 github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 ...'
            go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
            $goBin = Join-Path (go env GOPATH) 'bin'
            if ($env:Path -notlike "*$goBin*") {
                $env:Path = "$goBin;$env:Path"
            }
        }
        Write-Ok ((wails3 version) -join ' ')

        # Docker 守护进程
        $dockerVersion = docker version --format '{{.Server.Version}}' 2>$null
        if (-not $dockerVersion) {
            Fail @'
Docker 守护进程未运行。交叉编译依赖 Docker（Rancher Desktop / Docker Desktop）。
请先启动它，然后重新运行本脚本。
'@
        }
        Write-Ok "Docker Server $dockerVersion"

        # Docker 数据根位置 —— 交叉编译镜像会占用数 GB，不应落在 C 盘
        $dataRoot = (docker info --format '{{.DockerRootDir}}' 2>$null)
        if ($dataRoot) {
            Write-Ok "Docker 数据根: $dataRoot"
            $onSystemDrive = $dataRoot -match '^[Cc]:'
            if ($onSystemDrive -and -not $AllowDockerOnC) {
                Fail @"
Docker 数据根位于 C 盘：$dataRoot
wails-cross 镜像体积在 1GB 以上，会挤占系统盘。

请把镜像存储迁到 D 盘后重试：
  Rancher Desktop → Settings → Container Engine → Image store location → 选一个 D 盘目录
  （修改后需要重启 Rancher Desktop）

确认无碍时可加 -AllowDockerOnC 跳过此检查。
"@
            }
            if ($onSystemDrive) {
                Write-Warn 'Docker 数据根位于 C 盘，已通过 -AllowDockerOnC 放行。'
            }
        }
    }

    # ------------------------------------------------------------ 交叉编译
    $architectures = if ($Arch -eq 'both') { @('arm64', 'amd64') } else { @($Arch) }

    foreach ($targetArch in $architectures) {
        Write-Step "构建 darwin/$targetArch"

        $env:GOOS = 'darwin'
        $env:GOARCH = $targetArch

        # 交叉编译产物未签名，签名是独立步骤（wails3 sign），这里不做。
        wails3 build "GOOS=darwin" "GOARCH=$targetArch"
        if ($LASTEXITCODE -ne 0) {
            Fail "darwin/$targetArch 构建失败，退出码 $LASTEXITCODE"
        }
        Write-Ok "darwin/$targetArch 构建完成"
    }

    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue

    # ------------------------------------------------------------ 收集产物
    Write-Step '收集产物'

    $baseDir = if ([System.IO.Path]::IsPathRooted($OutputDir)) {
        $OutputDir
    } else {
        Join-Path $RepoRoot $OutputDir
    }

    foreach ($targetArch in $architectures) {
        # 构建两个架构时分别放进子目录，避免同名覆盖。
        $targetDir = if ($Arch -eq 'both') {
            Join-Path $baseDir $targetArch
        } else {
            $baseDir
        }
        New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

        $binary = Join-Path $RepoRoot 'bin/registry-browser'
        if (Test-Path $binary) {
            $destination = Join-Path $targetDir 'registry-browser'
            Copy-Item $binary -Destination $destination -Force
            Write-Ok "darwin/$targetArch -> $destination"
        } else {
            Write-Warn "没有找到 bin/registry-browser，跳过复制（产物可能输出在其它位置）"
        }
    }

    Write-Step '完成'
    Write-Host "产物目录: $baseDir" -ForegroundColor Green
    Write-Host '提醒：交叉编译产物未签名未公证，分发前需在 macOS 上执行 codesign + notarytool。' -ForegroundColor Yellow
}
finally {
    Pop-Location
}
