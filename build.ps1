# 重新编译 agentmover.exe
# 用法： .\build.ps1            编译（默认 windows/amd64，纯静态，无 CGO 依赖）
#        .\build.ps1 -All      同时产出 linux / darwin 版本
param(
    [switch]$All
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$stamp   = Get-Date -Format "yyyyMMdd-HHmm"
$ldflags = "-s -w -X main.buildTime=$stamp"

Write-Host "-> 编译 windows/amd64 ..." -ForegroundColor Cyan
$env:CGO_ENABLED = "0"
$env:GOOS   = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "$ldflags" -o agentmover.exe .
if ($LASTEXITCODE -ne 0) { throw "编译失败" }
$f = Get-Item .\agentmover.exe
Write-Host ("   OK  {0}  {1:N0} KB" -f $f.Name, ($f.Length / 1KB)) -ForegroundColor Green

if ($All) {
    New-Item -ItemType Directory -Force -Path dist | Out-Null
    foreach ($t in @(@("linux", "amd64"), @("darwin", "arm64"))) {
        $out = "dist\agentmover-$($t[0])-$($t[1])"
        Write-Host "-> 编译 $($t[0])/$($t[1]) ..." -ForegroundColor Cyan
        $env:GOOS = $t[0]; $env:GOARCH = $t[1]
        go build -trimpath -ldflags "$ldflags" -o $out .
        Write-Host "   OK  $out" -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "完成。运行 .\agentmover.exe" -ForegroundColor Green
