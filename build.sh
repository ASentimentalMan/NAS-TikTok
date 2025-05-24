#!/usr/bin/env bash

set -euo pipefail

# ==== 常量定义 ====
readonly REPO_URL="https://github.com/ASentimentalMan/NAS-TikTok.git"
readonly REPO_DIR="$HOME/workspace/NAS-TikTok"
readonly WORK_DIR="$HOME/workspace/pm2/NAS-TikTok"
readonly IMAGE_NAME="nas-tiktok"
readonly CONTAINER_NAME="nas-tiktok"
readonly DOCKER_BUILD_PROXY="http://172.17.0.1:1080"

# 初始化变量
TAG=""

# ==== 颜色 ====
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ==== 函数 ====
get_latest_semantic_version_tag() {
  echo -e "${BLUE}🔍 正在获取镜像 '${IMAGE_NAME}' 的标签列表...${NC}" >&2
  local tags
  tags=$(docker images --format '{{.Tag}}' "$IMAGE_NAME" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true)

  if [[ -z "$tags" ]]; then
    echo -e "${YELLOW}⚠️ 没有符合语义版本格式的标签 (vX.Y.Z)，将使用 v0.0.0 作为起点${NC}" >&2
    echo "v0.0.0"
    return
  fi

  echo "$tags" | sort -V | tail -n 1
}

increment_patch_version() {
  local version=$1
  local major minor patch
  IFS='.' read -r major minor patch <<<"${version#v}"
  patch=$((patch + 1))
  echo "v${major}.${minor}.${patch}"
}

build() {
  echo -e "${BLUE}🛠️ 开始检查必要目录及文件...${NC}"
  # 检查源代码目录是否存在，如果不存在则克隆，否则更新
  if [ ! -d "$REPO_DIR" ]; then
    echo -e "${YELLOW}⚠️ 源代码目录 '$REPO_DIR' 不存在${NC}"
    echo -e "${BLUE}🌐 正在从仓库拉取源代码...${NC}"
    if ! git clone --depth 1 $REPO_URL "$REPO_DIR"; then
      echo -e "${RED}❌ 克隆源代码失败${NC}" >&2
      exit 1
    fi
  # else
  #   # 更新源代码：使用 git fetch 和 git reset --hard 确保与远程仓库 main 分支完全同步
  #   echo -e "${BLUE}🌐 更新源代码...${NC}"
  #   cd "$REPO_DIR" || exit 1
  #   git fetch --all # 获取所有远程分支的最新状态
  #   # 重置本地分支到远程 main 分支的最新提交，丢弃本地所有未提交的更改
  #   git reset --hard origin/main
  fi

  echo -e "${YELLOW}📦 生成新的 Docker 镜像标签${NC}"
  local latest_tag
  latest_tag=$(get_latest_semantic_version_tag)
  TAG=$(increment_patch_version "$latest_tag")

  echo -e "${BLUE}🐳 开始构建镜像 ${IMAGE_NAME}:${TAG}${NC}"

  local build_args=()
  read -rp "是否使用代理？(y/n): " confirm
  if [[ "$confirm" =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}⚠️ 使用代理进行构建${NC}"
    build_args+=(--build-arg "HTTP_PROXY=$DOCKER_BUILD_PROXY" --build-arg "HTTPS_PROXY=$DOCKER_BUILD_PROXY")
  else
    echo -e "${YELLOW}⚠️ 不使用代理进行构建${NC}"
  fi

  docker build "${build_args[@]}" -t "${IMAGE_NAME}:${TAG}" "$REPO_DIR" || {
    echo -e "${RED}❌ 构建失败${NC}" >&2
    exit 1
  }

  echo -e "${GREEN}✅ 构建完成：${IMAGE_NAME}:${TAG}${NC}"

  if [[ "$latest_tag" != "v0.0.0" ]]; then
    read -rp "是否删除旧版本镜像 ${latest_tag}？(y/n): " delete_old
    [[ "$delete_old" =~ ^[Yy]$ ]] && docker rmi "${IMAGE_NAME}:${latest_tag}" || true
  fi

  # 导出镜像
  read -rp "是否导出镜像？(y/n): " export_img
  if [[ "$export_img" =~ ^[Yy]$ ]]; then
    export_docker_image
  fi
}

deploy() {
  if [ -z "$TAG" ]; then
    echo -e "${YELLOW}⚠️ 未指定 TAG，尝试使用最新版本...${NC}"
    TAG=$(get_latest_semantic_version_tag)
    if [ -z "$TAG" ]; then
      echo -e "${RED}❌ 未找到有效标签，请先构建镜像${NC}" >&2
      exit 1
    fi
    echo -e "${GREEN}✅ 使用最新标签：$TAG${NC}"
  fi

  if docker inspect "$CONTAINER_NAME" &>/dev/null; then
    echo -e "${BLUE}🛑 删除旧容器 $CONTAINER_NAME...${NC}"
    docker stop "$CONTAINER_NAME"
    docker rm "$CONTAINER_NAME"
  fi

  mkdir -p "$WORK_DIR/certs" "$WORK_DIR/statics"

  echo -e "${BLUE}📦 创建容器 ${CONTAINER_NAME}...${NC}"
  docker create \
    --name "$CONTAINER_NAME" \
    -v "$WORK_DIR/certs:/workspace/certs" \
    -v "$WORK_DIR/statics:/workspace/statics" \
    --restart unless-stopped \
    "${IMAGE_NAME}:${TAG}"

  read -rp "是否立即启动容器 '$CONTAINER_NAME'？(y/n): " confirm
  if [[ "$confirm" =~ ^[Yy]$ ]]; then
    docker start "$CONTAINER_NAME"
    echo -e "${GREEN}🟢 容器启动成功！${NC}"
  else
    echo -e "${YELLOW}⚠️ 用户取消启动容器${NC}"
    exit 0
  fi
}

export_docker_image() {
  echo -e "${BLUE}📦 准备导出镜像...${NC}"
  # 获取最新的语义版本标签
  TAG=$(get_latest_semantic_version_tag)

  if [ "$TAG" == "v0.0.0" ]; then
    echo -e "${RED}❌ 没有找到任何语义版本格式的镜像标签（vX.Y.Z），无法导出。请先构建镜像。${NC}" >&2
    exit 1
  fi

  local image_full_name="${IMAGE_NAME}:${TAG}"

  if ! docker images --format '{{.Repository}}:{{.Tag}}' | grep -q "^${image_full_name}$"; then
    echo -e "${RED}❌ 镜像 '${image_full_name}' 不存在，请检查标签或先构建镜像${NC}" >&2
    exit 1
  fi

  TAR_FILE="${IMAGE_NAME}_${TAG}.tar"
  echo -e "${BLUE}📦 正在导出最新镜像 '${image_full_name}' 为 ${TAR_FILE}...${NC}"
  if docker save -o "$TAR_FILE" "${image_full_name}"; then
    echo -e "${GREEN}✅ 镜像已成功导出为 ${TAR_FILE}${NC}"
  else
    echo -e "${RED}❌ 导出失败${NC}" >&2
    exit 1
  fi
}

import_docker_image() {
  echo -e "${BLUE}📦 准备从 '${REPO_DIR}' 导入最新 .tar 镜像文件...${NC}"

  # 查找 REPO_DIR 下最新的 .tar 文件
  local latest_tar_file
  latest_tar_file=$(find "$REPO_DIR" -maxdepth 1 -name "*.tar" -printf '%T@ %p\n' | sort -n | tail -1 | awk '{print $2}')

  if [ -z "$latest_tar_file" ]; then
    echo -e "${RED}❌ 在目录 '${REPO_DIR}' 中未找到任何 .tar 镜像文件。请确认文件是否存在。${NC}" >&2
    exit 1
  fi

  TAR_FILE="$latest_tar_file"
  echo -e "${BLUE}📦 正在导入文件：'${TAR_FILE}'...${NC}"
  if docker load -i "$TAR_FILE"; then
    echo -e "${GREEN}✅ 镜像已成功从 ${TAR_FILE} 导入${NC}"
    # 尝试从导入的镜像中获取最新标签
    # 查找以 IMAGE_NAME 开头且具有 vX.Y.Z 格式标签的最新镜像
    TAG=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep "^${IMAGE_NAME}:v[0-9]\+\.[0-9]\+\.[0-9]\+$" | sort -V | tail -n 1 | awk -F':' '{print $2}' || true)
    if [ -n "$TAG" ]; then
      echo -e "${GREEN}✅ 导入镜像的最新标签可能为: ${TAG}${NC}"
    else
      echo -e "${YELLOW}⚠️ 无法自动获取导入镜像的语义版本标签，请手动确认。${NC}"
    fi
  else
    echo -e "${RED}❌ 导入失败${NC}" >&2
    exit 1
  fi
}

# ==== 主逻辑 ====
if ! docker info >/dev/null 2>&1; then
  echo -e "${RED}❌ Docker 未运行，请先启动 Docker${NC}" >&2
  exit 1
fi

echo -e "${CYAN}当前配置：${NC}"
echo -e "${CYAN}  镜像名: ${BLUE}${IMAGE_NAME}${NC}"
echo -e "${CYAN}  容器名: ${BLUE}${CONTAINER_NAME}${NC}"
echo -e "${CYAN}  工作目录: ${BLUE}${WORK_DIR}${NC}"

read -rp "请选择操作:
  a. 构建
  b. 部署
  c. 构建 + 部署
  d. 导出镜像
  e. 导入镜像
  其它任意键退出...
> " choice

case "$choice" in
a | A)
  build
  ;;
b | B)
  deploy
  ;;
c | C)
  build && deploy
  ;;
d | D)
  export_docker_image
  ;;
e | E)
  import_docker_image
  ;;
*)
  echo -e "${RED}❌ 已取消操作${NC}"
  exit 0
  ;;
esac

echo -e "${GREEN}✅ 脚本执行完成！镜像版本: ${TAG}${NC}"
exit 0
