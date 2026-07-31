# PowerShell 交叉編譯腳本
$APP_NAME = "kcpvpn"
$OUTPUT_DIR = "bin"
$PLATFORMS = @(
    "linux/amd64",
    "linux/arm64",
    "linux/386",
    "windows/amd64",
    "windows/arm64",
    "windows/386",
    "darwin/amd64",
    "darwin/arm64"
)

go mod tidy

if (Test-Path $OUTPUT_DIR) {
    Remove-Item -Recurse -Force $OUTPUT_DIR
}
New-Item -ItemType Directory -Force -Path $OUTPUT_DIR | Out-Null

Write-Host "開始編譯 $APP_NAME ..."
Write-Host "------------------------------------------"

foreach ($PLATFORM in $PLATFORMS) {
    $parts = $PLATFORM.Split('/')
    $GOOS = $parts[0]
    $GOARCH = $parts[1]

    $OUTPUT_NAME = "${APP_NAME}_${GOOS}_${GOARCH}"
    if ($GOOS -eq "windows") {
        $OUTPUT_NAME += ".exe"
    }

    $OUTPUT_PATH = Join-Path $OUTPUT_DIR $OUTPUT_NAME

    Write-Host "正在編譯 -> 平台: $GOOS, 架構: $GOARCH"

    $env:CGO_ENABLED = "0"
    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH

    go build -ldflags="-s -w" -o $OUTPUT_PATH .
    if ($LASTEXITCODE -ne 0) {
        Write-Error "❌ 編譯失敗: $GOOS/$GOARCH"
        exit 1
    }
}

Write-Host "------------------------------------------"
Write-Host "✅ 所有編譯任務已完成！"
Write-Host "執行檔已保存至 '$OUTPUT_DIR' 目錄下。"
