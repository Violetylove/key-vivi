# 构建产物继承工作区的Low标签时，双击也会降为Low；在构建结束后修正单个exe。
$ErrorActionPreference = 'Stop'
$buildRoot = $PSScriptRoot
$buildOutput = Join-Path $buildRoot 'dist\KeyVivi.exe'
$previousCGO = $env:CGO_ENABLED
Push-Location -LiteralPath $buildRoot
try {
    $env:CGO_ENABLED = '0'
    & go build '-ldflags=-s -w -H=windowsgui' -o $buildOutput ./cmd/keyvivi
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }

    # 只设置本次产物为普通用户级别，不改变工作区目录的权限或完整性标签。
    & icacls $buildOutput /setintegritylevel M
    if ($LASTEXITCODE -ne 0) { throw 'Cannot prepare executable integrity label.' }
    Write-Host "Build ready: $buildOutput"
}
finally {
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
