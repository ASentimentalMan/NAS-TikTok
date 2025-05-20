#!/bin/sh

# 在运行时生成 env.js 文件
cat <<EOF > /workspace/client/dist/env.js
window.__APP_CONFIG__ = {
  VITE_API_URL: "${VITE_API_URL}",
  VITE_API_PORT: "${VITE_API_PORT}"
};
EOF

# 启动 PM2（保持原来的启动方式）
exec pm2-runtime start ecosystem.config.js