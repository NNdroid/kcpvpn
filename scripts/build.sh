#!/bin/bash

# ==========================================
# Go 多平台交叉編譯腳本
# ==========================================

# 自動切換到專案根目錄
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}" || exit 1

APP_NAME="kcpvpn"
MAIN_PATH="."
OUTPUT_DIR="bin"

# 定義需要編譯的目標平台 (格式: GOOS/GOARCH)
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "linux/386"
    "windows/amd64"
    "windows/arm64"
    "windows/386"
    "darwin/amd64"   # macOS Intel
    "darwin/arm64"   # macOS Apple Silicon (M1/M2/M3)
)

echo "🔄 執行 go mod tidy ..."
go mod tidy

echo "🧹 清理舊的編譯檔案..."
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

echo "🚀 開始編譯 ${APP_NAME} (多平台支持) ..."
echo "------------------------------------------"

for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS=${PLATFORM%/*}
    GOARCH=${PLATFORM#*/}
    
    OUTPUT_NAME="${APP_NAME}_${GOOS}_${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi
    
    OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_NAME}"
    
    echo "🔨 正在編譯 -> 平台: $GOOS, 架構: $GOARCH"
    
    env CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags="-s -w" -o "${OUTPUT_PATH}" "${MAIN_PATH}"
    
    if [ $? -ne 0 ]; then
        echo "❌ 編譯失敗: $GOOS/$GOARCH"
        exit 1
    fi
done

echo "------------------------------------------"
echo "✅ 所有編譯任務已完成！"
echo "📂 執行檔已保存至 '${OUTPUT_DIR}' 目錄下："
ls -lh "${OUTPUT_DIR}"