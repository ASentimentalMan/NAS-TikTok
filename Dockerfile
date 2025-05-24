# --- Client Builder Stage ---
FROM node:22-alpine AS client-builder

# 设置代理
ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY="localhost,127.0.0.1,::1,172.17.0.1"

WORKDIR /app/client

# 复制客户端依赖文件
COPY client/package*.json ./
RUN npm ci --no-audit --no-fund

# 复制客户端源代码并构建
COPY client/ ./
RUN npm run build && \
  npm cache clean --force

# --- Server Builder Stage ---
FROM node:22-alpine AS server-builder

# 设置代理
ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY="localhost,127.0.0.1,::1,172.17.0.1"

WORKDIR /app/server

# 复制服务端依赖文件
COPY server/package*.json ./
RUN npm ci --no-audit --no-fund

# 复制服务端源代码并构建
COPY server/ ./
RUN npm run build && \
  npm prune --omit=dev && \
  npm cache clean --force

# --- Runtime Stage ---
FROM node:22-alpine

WORKDIR /workspace

# 设置生产环境变量
ENV NODE_ENV=production
ENV SSL=
ENV LAN_ONLY=true
ENV IPv6=false
ENV VITE_API_URL=
ENV VITE_API_PORT=
ENV ACCOUNT=
ENV PASSWORD=
ENV JWT_SECRET=
ENV JWT_EXPIRE=1200

# 安装PM2
RUN npm install -g pm2@latest --no-audit --no-fund && \
  npm cache clean --force

# 从client-builder复制客户端构建产物
COPY --from=client-builder /app/client/dist /workspace/client/dist/

# 从server-builder复制服务端文件
COPY --from=server-builder /app/server/.env* /workspace/server/
COPY --from=server-builder \
  /app/server/ecosystem.config.js \
  /app/server/entrypoint.sh \
  /workspace/

# 复制生产依赖和构建产物
COPY --from=server-builder /app/server/node_modules /workspace/server/node_modules
COPY --from=server-builder /app/server/dist /workspace/server/dist/

# 设置权限
RUN chmod +x /workspace/entrypoint.sh

ENTRYPOINT ["/workspace/entrypoint.sh"]