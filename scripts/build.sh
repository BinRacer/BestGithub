#!/bin/bash

# 定义目标平台和架构列表
platforms=("linux/amd64" "windows/amd64" "darwin/amd64")

# 获取脚本所在目录的绝对路径，并计算项目根目录和src目录路径
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SRC_DIR="$PROJECT_ROOT/src"
DIST_DIR="$PROJECT_ROOT/dist"

# 创建dist目录（如果不存在）
mkdir -p "$DIST_DIR"

# 遍历所有平台
for platform in "${platforms[@]}"
do
  # 提取操作系统和架构
  OS=$(echo "$platform" | cut -d'/' -f1)
  ARCH=$(echo "$platform" | cut -d'/' -f2)
  output_name="BestGithub-${OS}-${ARCH}"

  # 如果是Windows平台，添加.exe扩展名
  if [ "$OS" = "windows" ]; then
    output_name="${output_name}.exe"
  fi

  # 创建平台特定子目录（例如dist/linux）
  PLATFORM_DIR="$DIST_DIR/${OS}"
  mkdir -p "$PLATFORM_DIR"

  # 设置环境变量并以Release方式构建，在src目录中运行go build，输出文件到平台目录
  (cd "$SRC_DIR" && env GOOS="$OS" GOARCH="$ARCH" go build -ldflags="-s -w" -o "$PLATFORM_DIR/$output_name")

  # 检查构建是否成功
  if [ $? -eq 0 ]; then
    echo "成功构建用于 $OS/$ARCH: $PLATFORM_DIR/$output_name"
  else
    echo "构建失败用于 $OS/$ARCH"
  fi
done
