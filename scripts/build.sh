#!/bin/bash

# ==========================================
# Go 交叉編譯腳本
# ==========================================

# 1. 設定你的應用程式名稱
APP_NAME="kcpvpn"

# 2. 設定編譯的入口文件或目錄 (通常是 main.go 或 .)
MAIN_PATH="."

# 3. 輸出目錄
OUTPUT_DIR="bin"

# 4. 定義需要編譯的目標平台 (格式: GOOS/GOARCH)
# 你可以根據需求增刪這裡的陣列內容
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

go mod tidy
# 清理並重新創建輸出目錄
echo "清理舊的編譯檔案..."
rm -rf ${OUTPUT_DIR}
mkdir -p ${OUTPUT_DIR}

echo "開始編譯 ${APP_NAME} ..."
echo "------------------------------------------"

# 遍歷所有目標平台進行編譯
for PLATFORM in "${PLATFORMS[@]}"; do
    # 使用字串截取分離 GOOS 和 GOARCH
    GOOS=${PLATFORM%/*}
    GOARCH=${PLATFORM#*/}
    
    # 定義輸出的檔案名稱
    OUTPUT_NAME="${APP_NAME}_${GOOS}_${GOARCH}"
    
    # 如果是 Windows 系統，自動加上 .exe 副檔名
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi
    
    # 組合完整的輸出路徑
    OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_NAME}"
    
    echo "正在編譯 -> 平台: $GOOS, 架構: $GOARCH"
    
    # 執行 go build
    # CGO_ENABLED=0 通常用於靜態編譯，確保在不同環境下的相容性
    env CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags="-s -w" -o ${OUTPUT_PATH} ${MAIN_PATH}
    
    # 檢查編譯是否成功
    if [ $? -ne 0 ]; then
        echo "❌ 編譯失敗: $GOOS/$GOARCH"
        exit 1
    fi
done

echo "------------------------------------------"
echo "✅ 所有編譯任務已完成！"
echo "執行檔已保存至 '${OUTPUT_DIR}' 目錄下。"