# --- Builder Stage ---
FROM node:22-alpine AS builder

# 设置代理（构建阶段使用）
ARG HTTP_PROXY=
ARG HTTPS_PROXY=

ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY="localhost,127.0.0.1,::1,172.17.0.1"

# 设置工作目录
WORKDIR /app

# 安装客户端依赖
WORKDIR /app/client
COPY client/package.json ./
RUN npm install

# 安装服务端依赖
WORKDIR /app/server
COPY server/package.json ./
RUN npm install

# 复制所有源代码
WORKDIR /app
COPY . .

# 构建客户端
WORKDIR /app/client
RUN npm run build

# 构建服务端
WORKDIR /app/server
RUN npm run build && \
  npm prune --omit=dev

# 取消代理
ENV HTTP_PROXY=""
ENV HTTPS_PROXY=""
ENV NO_PROXY=""

# --- Runtime Stage ---
FROM node:22-alpine

WORKDIR /workspace

ENV NODE_ENV=production
ENV SSL=
ENV LAN_ONLY=
ENV ACCOUNT=
ENV PASSWORD=
ENV APP_HOST=
ENV VITE_API_URL=
ENV VITE_API_PORT=
ENV JWT_SECRET=
ENV JWT_EXPIRE=1200

# 安装 PM2
RUN npm install -g pm2

# 拷贝配置文件
COPY --from=builder /app/server/.env* /workspace/server/
COPY --from=builder \
  /app/server/ecosystem.config.js \
  /app/server/entrypoint.sh \
  /workspace/

# 拷贝 node_modules
COPY --from=builder /app/server/node_modules /workspace/server/node_modules

# 拷贝构建产物
COPY --from=builder /app/client/dist /workspace/client/dist/
COPY --from=builder /app/server/dist /workspace/server/dist/

RUN chmod +x /workspace/entrypoint.sh

ENTRYPOINT ["/workspace/entrypoint.sh"]
