#!/usr/bin/env bash

set -euo pipefail

# ==== 常量定义 ====
readonly REPO_URL="https://github.com/ASentimentalMan/NAS-TikTok.git"
readonly REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly WORK_DIR="$HOME/workspace/pm2/NAS-TikTok"
readonly ENV_FILE="$WORK_DIR/.env"
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

wait_for_healthy() {
  local container_name=$1 timeout_seconds=90 elapsed=0 health status
  while ((elapsed < timeout_seconds)); do
    health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container_name" 2>/dev/null || true)
    case "$health" in
      healthy) return 0 ;;
      unhealthy | missing) return 1 ;;
    esac

    status=$(docker inspect --format '{{.State.Status}}' "$container_name" 2>/dev/null || true)
    [[ "$status" == "running" ]] || return 1
    sleep 3
    elapsed=$((elapsed + 3))
  done
  return 1
}

rollback_deployment() {
  local candidate_name=$1 previous_name=$2 candidate_renamed=$3 previous_moved=$4 previous_was_running=$5
  local failed_name=$candidate_name rollback_failed=0 status
  trap '' INT TERM HUP QUIT
  [[ "$candidate_renamed" == "true" ]] && failed_name=$CONTAINER_NAME

  if docker inspect "$failed_name" &>/dev/null; then
    docker rm -f "$failed_name" >/dev/null || rollback_failed=1
  fi

  if [[ "$previous_moved" == "true" ]]; then
    if docker rename "$previous_name" "$CONTAINER_NAME"; then
      if [[ "$previous_was_running" == "true" ]] && ! docker start "$CONTAINER_NAME" >/dev/null; then
        rollback_failed=1
      fi
    else
      rollback_failed=1
    fi
  elif [[ "$previous_was_running" == "true" ]] && docker inspect "$CONTAINER_NAME" &>/dev/null; then
    status=$(docker inspect --format '{{.State.Status}}' "$CONTAINER_NAME" 2>/dev/null || true)
    if [[ "$status" != "running" ]] && ! docker start "$CONTAINER_NAME" >/dev/null; then
      rollback_failed=1
    fi
  fi

  if ((rollback_failed == 0)); then
    echo -e "${GREEN}↩️ 已恢复原容器状态${NC}"
    return 0
  fi
  echo -e "${RED}❌ 自动恢复未能完整完成。请检查容器 '$CONTAINER_NAME' 与 '$previous_name' 的状态${NC}" >&2
  return 1
}

handle_deploy_signal() {
  local candidate_name=$1 previous_name=$2 candidate_renamed=$3 previous_moved=$4 previous_was_running=$5
  local old_container_exists=$6 exit_status=$7

  # Docker may complete a rename just before Bash runs the following flag assignment.
  if docker inspect "$previous_name" &>/dev/null; then
    previous_moved=true
    if docker inspect "$CONTAINER_NAME" &>/dev/null; then
      candidate_renamed=true
    fi
  elif [[ "$old_container_exists" == "false" ]] && docker inspect "$CONTAINER_NAME" &>/dev/null; then
    candidate_renamed=true
  fi

  rollback_deployment "$candidate_name" "$previous_name" "$candidate_renamed" "$previous_moved" "$previous_was_running" || true
  exit "$exit_status"
}

build() {
  echo -e "${BLUE}🛠️ 开始检查必要目录及文件...${NC}"
  local build_revision
  if ! build_revision=$(git -C "$REPO_DIR" rev-parse --verify HEAD 2>/dev/null); then
    echo -e "${RED}❌ 源码目录不是 Git 工作区，无法记录构建 Commit${NC}" >&2
    exit 1
  fi
  if [[ -n "$(git -C "$REPO_DIR" status --porcelain)" ]]; then
    build_revision+="-dirty"
    echo -e "${YELLOW}⚠️ 当前工作区有未提交改动，镜像将标记为 dirty${NC}"
  fi
  echo -e "${CYAN}📌 构建源码 Commit: ${build_revision}${NC}"

  echo -e "${YELLOW}📦 生成新的 Docker 镜像标签${NC}"
  local latest_tag
  latest_tag=$(get_latest_semantic_version_tag)
  TAG=$(increment_patch_version "$latest_tag")

  echo -e "${BLUE}🐳 开始构建镜像 ${IMAGE_NAME}:${TAG}${NC}"

  local build_args=()
  build_args+=(--build-arg "VITE_BASE_URL=${VITE_BASE_URL:-/}")
  read -rp "是否使用代理？(y/n): " confirm
  if [[ "$confirm" =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}⚠️ 使用代理进行构建${NC}"
    build_args+=(--build-arg "HTTP_PROXY=$DOCKER_BUILD_PROXY" --build-arg "HTTPS_PROXY=$DOCKER_BUILD_PROXY")
  else
    echo -e "${YELLOW}⚠️ 不使用代理进行构建${NC}"
  fi

  docker build "${build_args[@]}" \
    --label "org.opencontainers.image.source=$REPO_URL" \
    --label "org.opencontainers.image.revision=$build_revision" \
    -t "${IMAGE_NAME}:${TAG}" "$REPO_DIR" || {
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

  if [[ ! -r "$ENV_FILE" ]]; then
    echo -e "${RED}❌ 缺少运行时环境文件 '$ENV_FILE'，请先创建并填入所需配置${NC}" >&2
    exit 1
  fi

  local network_mode listen_port port_number
  read -rp "请选择网络模式 (b=bridge, h=host，默认 b): " network_mode
  case "${network_mode,,}" in
    "" | b | bridge) network_mode="bridge" ;;
    h | host) network_mode="host" ;;
    *)
      echo -e "${RED}❌ 网络模式只能是 bridge 或 host${NC}" >&2
      exit 1
      ;;
  esac

  read -rp "Go 服务监听端口 (默认 3000): " listen_port
  listen_port=${listen_port:-3000}
  if [[ ! "$listen_port" =~ ^[0-9]+$ ]]; then
    echo -e "${RED}❌ 端口必须是 1 到 65535 之间的整数${NC}" >&2
    exit 1
  fi
  port_number=$((10#$listen_port))
  if ((port_number < 1 || port_number > 65535)); then
    echo -e "${RED}❌ 端口必须是 1 到 65535 之间的整数${NC}" >&2
    exit 1
  fi
  listen_port=$port_number

  local candidate_name="${CONTAINER_NAME}-candidate" previous_name="${CONTAINER_NAME}-previous"
  local old_container_exists=false old_was_running=false old_status tls_enabled tls_default_paths
  local candidate_renamed=false previous_moved=false

  if ! docker image inspect "${IMAGE_NAME}:${TAG}" &>/dev/null; then
    echo -e "${RED}❌ 本地镜像 '${IMAGE_NAME}:${TAG}' 不存在${NC}" >&2
    exit 1
  fi
  if docker inspect "$candidate_name" &>/dev/null || docker inspect "$previous_name" &>/dev/null; then
    echo -e "${RED}❌ 检测到临时容器 '$candidate_name' 或 '$previous_name'，请先人工检查其状态${NC}" >&2
    exit 1
  fi

  if docker inspect "$CONTAINER_NAME" &>/dev/null; then
    old_container_exists=true
    old_status=$(docker inspect --format '{{.State.Status}}' "$CONTAINER_NAME")
    case "$old_status" in
      running) old_was_running=true ;;
      exited | created | dead) ;;
      *)
        echo -e "${RED}❌ 旧容器状态为 '$old_status'，请先人工检查${NC}" >&2
        exit 1
        ;;
    esac
  fi

  if [[ "$old_container_exists" == "false" ]] && command -v ss >/dev/null 2>&1; then
    local listening_sockets
    listening_sockets=$(ss -H -ltn "sport = :$listen_port" 2>/dev/null || true)
    if [[ -n "$listening_sockets" ]]; then
      echo -e "${RED}❌ TCP 端口 $listen_port 已被占用${NC}" >&2
      exit 1
    fi
  elif [[ "$old_container_exists" == "true" ]]; then
    echo -e "${YELLOW}ℹ️ 保留旧容器时跳过本机端口占用探测；新容器启动失败会触发回滚${NC}"
  fi

  read -rp "运行时 .env 是否启用 SSL=true？(y/n): " tls_enabled
  if [[ "${tls_enabled,,}" =~ ^(y|yes)$ ]]; then
    read -rp "是否使用默认证书路径 certs/fullchain.pem 与 certs/privkey.pem？(y/n): " tls_default_paths
    if [[ "${tls_default_paths,,}" =~ ^(y|yes)$ ]]; then
      if [[ ! -s "$WORK_DIR/certs/fullchain.pem" || ! -r "$WORK_DIR/certs/fullchain.pem" || ! -s "$WORK_DIR/certs/privkey.pem" || ! -r "$WORK_DIR/certs/privkey.pem" ]]; then
        echo -e "${RED}❌ 默认 TLS 证书与私钥必须存在、非空且对当前用户可读${NC}" >&2
        exit 1
      fi
    else
      echo -e "${YELLOW}⚠️ 自定义 TLS 路径不做文件预检；容器健康检查会验证服务能否启动${NC}"
    fi
  fi

  read -rp "确认部署 ${IMAGE_NAME}:${TAG} 并启动服务？(y/n): " confirm
  if [[ ! "${confirm,,}" =~ ^(y|yes)$ ]]; then
    echo -e "${YELLOW}⚠️ 已取消，现有容器未变更${NC}"
    exit 0
  fi

  mkdir -p "$WORK_DIR/certs" "$WORK_DIR/statics" "$WORK_DIR/db"
  if ! docker run --rm --user 0:0 \
    -v "$WORK_DIR/db:/workspace/db" \
    --entrypoint chown "${IMAGE_NAME}:${TAG}" -R app:app /workspace/db; then
    echo -e "${RED}❌ 无法为应用用户设置数据库目录写入权限${NC}" >&2
    exit 1
  fi

  local network_args=()
  if [[ "$network_mode" == "bridge" ]]; then
    network_args+=(--publish "$listen_port:$listen_port")
  else
    network_args+=(--network host)
  fi

  echo -e "${BLUE}📦 预创建候选 Go 容器 ${candidate_name} (${network_mode})...${NC}"
  trap 'handle_deploy_signal "$candidate_name" "$previous_name" "$candidate_renamed" "$previous_moved" "$old_was_running" "$old_container_exists" 129' HUP
  trap 'handle_deploy_signal "$candidate_name" "$previous_name" "$candidate_renamed" "$previous_moved" "$old_was_running" "$old_container_exists" 130' INT
  trap 'handle_deploy_signal "$candidate_name" "$previous_name" "$candidate_renamed" "$previous_moved" "$old_was_running" "$old_container_exists" 131' QUIT
  trap 'handle_deploy_signal "$candidate_name" "$previous_name" "$candidate_renamed" "$previous_moved" "$old_was_running" "$old_container_exists" 143' TERM
  if ! docker create \
    --name "$candidate_name" \
    --env-file "$ENV_FILE" \
    --env "LISTEN_PORT=$listen_port" \
    --env "VITE_API_PORT=$listen_port" \
    --env MEDIA_ROOT=/workspace/statics \
    --env DB_PATH=/workspace/db/nas-tiktok.db \
    --env STATIC_ROOT=/workspace/client/dist \
    "${network_args[@]}" \
    -v "$WORK_DIR/certs:/workspace/certs:ro" \
    -v "$WORK_DIR/statics:/workspace/statics:ro" \
    -v "$WORK_DIR/db:/workspace/db" \
    --restart unless-stopped \
    "${IMAGE_NAME}:${TAG}" >/dev/null; then
    if docker inspect "$candidate_name" &>/dev/null; then
      docker rm -f "$candidate_name" >/dev/null || true
    fi
    echo -e "${RED}❌ 候选容器创建失败，现有容器未变更${NC}" >&2
    exit 1
  fi

  if [[ "$old_container_exists" == "true" ]]; then
    if [[ "$old_was_running" == "true" ]] && ! docker stop "$CONTAINER_NAME" >/dev/null; then
      rollback_deployment "$candidate_name" "$previous_name" false false "$old_was_running" || true
      echo -e "${RED}❌ 无法停止旧容器，已取消部署${NC}" >&2
      exit 1
    fi
    if ! docker rename "$CONTAINER_NAME" "$previous_name"; then
      rollback_deployment "$candidate_name" "$previous_name" false false "$old_was_running" || true
      echo -e "${RED}❌ 无法暂存旧容器，已取消部署${NC}" >&2
      exit 1
    fi
    previous_moved=true
  fi

  if ! docker rename "$candidate_name" "$CONTAINER_NAME"; then
    rollback_deployment "$candidate_name" "$previous_name" false "$previous_moved" "$old_was_running" || true
    echo -e "${RED}❌ 无法切换候选容器名称${NC}" >&2
    exit 1
  fi
  candidate_renamed=true

  echo -e "${BLUE}🚀 启动容器并等待本地健康检查...${NC}"
  if ! docker start "$CONTAINER_NAME" >/dev/null || ! wait_for_healthy "$CONTAINER_NAME"; then
    echo -e "${RED}❌ 新容器未能通过健康检查（最多等待 90 秒），正在恢复旧容器${NC}" >&2
    rollback_deployment "$candidate_name" "$previous_name" "$candidate_renamed" "$previous_moved" "$old_was_running" || true
    exit 1
  fi

  trap - INT TERM HUP QUIT

  if [[ "$previous_moved" == "true" ]]; then
    if docker rm "$previous_name" >/dev/null; then
      echo -e "${GREEN}🗑️ 已移除确认健康后的旧容器${NC}"
    else
      echo -e "${YELLOW}⚠️ 新服务健康，但旧容器 '$previous_name' 未能清理，请稍后人工检查${NC}"
    fi
  fi
  echo -e "${GREEN}🟢 容器已启动并通过本地健康检查${NC}"
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
