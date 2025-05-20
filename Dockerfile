# --- Builder Stage ---
FROM node:22-alpine AS builder

# 设置代理（构建阶段使用）
ENV HTTP_PROXY=""
ENV HTTPS_PROXY=""

# 设置工作目录
WORKDIR /app

# 只复制 package.json 安装依赖
COPY server/package.json ./server/
COPY client/package.json ./client/

# 安装 server 依赖
WORKDIR /app/server
RUN npm install

# 安装 client 依赖
WORKDIR /app/client
RUN npm install

# 复制所有源代码
WORKDIR /app
COPY . .

# 构建 server
WORKDIR /app/server
RUN npm run build

# 构建 client
WORKDIR /app/client
RUN npm run build

# 取消代理
ENV HTTP_PROXY=""
ENV HTTPS_PROXY=""

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

# 拷贝构建产物
COPY --from=builder /app/client/dist /workspace/client/dist/
COPY --from=builder /app/server/dist /workspace/server/dist/

# 拷贝配置文件
COPY --from=builder /app/server/.env* /workspace/server/
COPY --from=builder /app/server/ecosystem.config.js /workspace/
COPY --from=builder /app/server/entrypoint.sh /workspace/

RUN chmod +x /workspace/entrypoint.sh

ENTRYPOINT ["/workspace/entrypoint.sh"]
