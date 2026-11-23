#!/bin/bash

# ==========================================
# Go 多平台交叉編譯腳本 (兼容 Windows / Linux / macOS)
# ==========================================
#
# 環境差異說明:
#   在 Windows 的 Git Bash / MSYS 環境下，直接執行 `go build -o <path>`
#   會靜默返回成功(exit 0)但不產出二進位檔案(這是 MSYS 與原生 go.exe
#   互動的已知問題)。因此偵測到 MSYS/MINGW/CYGWIN 時，改用 `cmd //c`
#   包裝 go build 呼叫，確保輸出檔案真正寫入磁碟。
#   Linux / macOS 則直接原生呼叫 go build。

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

# 偵測是否在 Windows 的 MSYS/Git Bash 環境
UNAME_S="$(uname -s 2>/dev/null || echo unknown)"
IS_MSYS=0
case "${UNAME_S}" in
    MINGW*|MSYS*|CYGWIN*) IS_MSYS=1 ;;
esac

# Windows (MSYS) 下以 cmd 執行臨時 .bat 檔來呼叫 go build，否則原生執行。
# 原因: Bash -> cmd 的引數傳遞會破壞 -ldflags 內的引號，故改用 .bat 檔
# 讓 cmd 以原生規則解析引號。回傳值為 go build 的退出碼。
run_go_build() {
    local goos="$1" goarch="$2" out_path="$3"
    if [ "${IS_MSYS}" -eq 1 ]; then
        # 轉成 Windows 絕對路徑供 cmd 使用 (E:/... 形式 cmd 可接受)
        local win_out
        win_out="$(cygpath -m "${out_path}" 2>/dev/null || echo "${out_path}")"
        local bat_file
        bat_file="$(mktemp --suffix=.bat 2>/dev/null || mktemp).bat"
        cat > "${bat_file}" <<BAT
@echo off
set CGO_ENABLED=0
set GOOS=${goos}
set GOARCH=${goarch}
go build -ldflags="-s -w -X main.version=${VERSION}" -o "${win_out}" "${MAIN_PATH}"
BAT
        cmd //c "${bat_file}"
        local rc=$?
        rm -f "${bat_file}"
        return ${rc}
    else
        env CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
            go build -ldflags="-s -w -X main.version=${VERSION}" \
            -o "${out_path}" "${MAIN_PATH}"
    fi
}

echo "🔄 執行 go mod tidy ..."
go mod tidy

# 依 git 歷史計算版本號: v1.0.<last commit yyyyMMdd>-<hash7>-<tagCount+1>
GIT_DATE=$(git log -1 --format='%ad' --date=format:'%Y%m%d' 2>/dev/null)
GIT_HASH=$(git log -1 --format='%h' 2>/dev/null)
GIT_TAG_COUNT=$(git tag 2>/dev/null | wc -l | tr -d ' ')
if [ -n "$GIT_DATE" ] && [ -n "$GIT_HASH" ]; then
    VERSION="v1.0.${GIT_DATE}-${GIT_HASH}-$((GIT_TAG_COUNT + 1))"
else
    VERSION="dev"
fi
echo "📌 版本號: ${VERSION}"

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

    # 使用絕對路徑輸出，避免 Windows/Git Bash 對相對路徑的轉換問題。
    OUTPUT_PATH="${ROOT_DIR}/${OUTPUT_DIR}/${OUTPUT_NAME}"

    echo "🔨 正在編譯 -> 平台: $GOOS, 架構: $GOARCH"

    if ! run_go_build "${GOOS}" "${GOARCH}" "${OUTPUT_PATH}"; then
        echo "❌ 編譯失敗: $GOOS/$GOARCH"
        exit 1
    fi

    # 驗證輸出檔案確實存在 (防止 MSYS 下靜默失敗卻回傳 0 的情況)
    if [ ! -f "${OUTPUT_PATH}" ]; then
        echo "❌ 輸出檔案未生成 (疑似環境問題): ${OUTPUT_PATH}"
        exit 1
    fi
done

echo "------------------------------------------"
echo "✅ 所有編譯任務已完成！"
echo "📂 執行檔已保存至 '${OUTPUT_DIR}' 目錄下："
ls -lh "${ROOT_DIR}/${OUTPUT_DIR}"
