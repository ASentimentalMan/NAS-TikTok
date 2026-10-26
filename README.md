<h1 align="center">
  NAS-TikTok
</h1>

<p align="center">
 <a href="README.md">简体中文</a> | <a href="README_en.md">English</a>
</p>

<p align="center">
<a><img src="https://img.shields.io/badge/license-GPLv3-orange"/></a>
<a><img src="https://img.shields.io/badge/PRs-welcome-brightgreen"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-Go-blue"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-Vue.js-blue"/></a>

</p>

你是否因为 NAS 资源太多而不知该看哪个？现在 `NAS-TikTok` 来帮你解决这个烦恼了！`NAS-TikTok` 是一个前端模仿 `抖音` ，后端对资源库进行随机抽样的项目。让你拥有刷抖音一样浏览 NAS 资源的丝滑流畅体验！

## 技术栈

### 后端 Go

1. 提供单进程 API、登录鉴权、验证码和媒体流服务。

2. 使用 SQLite 持久化媒体索引；索引未就绪时回退到硬盘扫描。

3. 可配置 HTTP 或 HTTPS；HTTPS 需要自备证书。

4. 支持配置 IP 白名单和 IPv6 监听。

### 前端 Vue.js

1. 支持随机跳转影片时间（即使滑到相同影片，播放的片段也不同）

2. 支持 2 倍速播放 / 视频旋转 功能

3. 支持图片查看 / 放大 / 旋转 / 自动播放 功能

4. 默认隐藏操作按钮，触摸时才显示，不占据视野

5. 支持无缓 PWA：iOS 设备可以从 Safari 添加到桌面（安卓未测试）当作 APP 使用，且设备上不缓存任何数据

## 在线预览

[NAS-Tiktok Preview](https://dev.engraved.cn/nas-tiktok)

账号：preview

密码：preview

## 运行

注意：本项目仅适用于学习和研究，不得用于商业使用。

### 本地开发

---

本项目的后端由 Go 提供。Node.js/npm 用于 Vue 前端开发和构建。需要 Go 1.24 或更新版本，以及 Node.js 22.23.3（与项目 CI 使用的版本一致）。

先克隆项目并创建媒体目录：

```bash
git clone https://github.com/ASentimentalMan/NAS-TikTok.git
cd NAS-TikTok
mkdir -p static
```

将要浏览的视频和图片放入 `static/`。在项目根目录打开第一个终端，启动 Go 后端；保持该终端运行：

```bash
cd server
ACCOUNT=demo \
PASSWORD=change-me \
JWT_SECRET='local-development-only-secret-replace-before-sharing' \
APP_URL_PREFIX= \
API_BASE=/api \
LISTEN_PORT=3000 \
MEDIA_ROOT=../static \
go run ./cmd/nas-tiktok
```

本地运行时可设置 `APP_ENV=development` 加载 `server/.env.development`；进程环境变量优先于文件中的值。新配置可参考 `server/.env.example`。

在项目根目录另开第二个终端，启动前端：

```bash
cd client
npm ci
npm run dev
```

Vite 会把 `/api` 请求转发到后端。打开 [http://127.0.0.1:5173](http://127.0.0.1:5173)，使用账号 `demo`、密码 `change-me` 登录。若终端没有从项目根目录启动，请先切换到克隆出的 `NAS-TikTok` 目录。示例密钥仅用于本机开发；共享或部署前请换成至少 32 字节的随机密钥，不要提交真实凭据。

Go 服务常用配置：`LISTEN_HOST`（默认 `0.0.0.0`）、`LISTEN_PORT`（默认 `3000`，未设置时也可由 `VITE_API_PORT` 提供）、`MEDIA_ROOT`（默认 `server/static`）、`STATIC_ROOT`（默认 `client/dist`）、`APP_URL_PREFIX`（可选部署路径）、`API_BASE`（可选前端 API 基址，默认同源 `/api`）、`SSL`（默认 `false`）、`TLS_CERT_FILE` 和 `TLS_KEY_FILE`（HTTPS 证书路径，默认 `certs/fullchain.pem` 与 `certs/privkey.pem`）、`ALLOWED_IPS`（默认未设置或为空；只控制来源 IP 白名单，空值不限制来源 IP，不影响账号密码登录或其他安全校验；填写 IP/CIDR 后仅允许匹配来源）、`ALLOW_IPv6`、`ACCOUNT`、`PASSWORD`、`JWT_SECRET` 与 `JWT_EXPIRE`（默认 1200 秒）。`MEDIA_ROOT` 和 `STATIC_ROOT` 的相对默认路径以 Go 进程的当前工作目录为基准；启动时会解析为绝对路径。本地示例从 `server/` 启动，因此显式把媒体目录设为 `../static`；若由 Go 服务托管前端，默认静态目录会是 `server/client/dist`。Docker 镜像已将这两个目录设为绝对路径。`COOKIE_SAME_SITE` 默认 `lax`，`COOKIE_SECURE` 默认跟随 `SSL`。跨站 Cookie 需要在 `CORS_ORIGINS` 中列出前端来源，并设置 `COOKIE_SAME_SITE=none`、`COOKIE_SECURE=true`；浏览器跨站使用还需要 HTTPS。只有配置了 `TRUSTED_PROXIES` 的代理网段，服务才会信任代理转发的客户端 IP 头。`ACCOUNT`、`PASSWORD`、`JWT_SECRET` 必须在运行时配置；签名密钥至少 32 字节。启用 HTTPS 时，证书和私钥必须可读。根路径部署无需额外配置；部署到子路径时，运行时设置 `APP_URL_PREFIX`，构建时设置匹配的 `VITE_BASE_URL`（例如 `/photos/`）。

`LOG_LEVEL` 默认为 `info`，记录登录结果和索引初始化、恢复、完成或失败状态；`debug` 还记录脱敏请求诊断及逐目录扫描进度；`off` 关闭常规运行日志，但关键错误仍会输出以便诊断。同源 HTTPS 反向代理无需默认放宽 `CORS_ORIGINS`：Go 收到的 Host 必须保留外部主机和端口；如果 Go 上游连接使用 HTTP，`TRUSTED_PROXIES` 必须匹配 Go 实际看到的直接代理地址，且代理必须传递真实外部协议。多层代理应根据实际 TLS 终止位置核对 `X-Forwarded-Proto`。

`DB_PATH` 默认是 `db/nas-tiktok.db`，相对路径以 Go 进程的当前工作目录为基准。媒体列表在 SQLite 索引就绪时从数据库抽样；首次建索引或重建期间会回退到硬盘扫描。添加或移除 NAS 文件后，可使用已登录会话调用 `GET {APP_URL_PREFIX}/api/reinit`（根路径为 `/api/reinit`）启动后台重建；请求无需正文，成功启动时立即返回 `202`，已有重建任务运行时返回 `409`。

### 正式部署

---

#### Docker 内部结构

```bash
/workspace
├── certs/ # 证书文件
│   ├── fullchain.pem # 证书
│   └── privkey.pem # 私钥
├── db/ # 持久化 SQLite 媒体索引
├── static/ # 只读媒体目录
└── client/dist/ # Docker 构建阶段生成的 Vue 静态文件
/usr/local/bin/nas-tiktok # Go 单进程服务
```

#### 本地构建与部署

```bash
docker build -t nas-tiktok:local .
```

`Dockerfile` 在构建阶段用 Node.js 生成 Vue `client/dist`，再用 Go 1.24 构建后端；最终镜像由 Go 进程同时提供 API 和前端静态文件。账号、密码和 JWT 密钥通过运行时环境提供，不复制进最终镜像。本地 Docker 镜像构建及使用临时媒体库和持久化数据库目录的容器启动已验证；真实 NAS 挂载与生产部署流程未验证。

构建需要启用 BuildKit，因为 Dockerfile 使用跨构建阶段的只读挂载；启动已导出的镜像 tar 不需要 BuildKit。

部署时，SQLite 数据库保存在 `$HOME/workspace/NAS-TikTok/db/`，并以读写方式挂载到容器的 `/workspace/db`。媒体和证书目录仍以只读方式挂载。

新镜像首次启动时，初始化入口读取 Docker 实际传入的 UID:GID，为容器自有的 `/workspace` 树（包括 `db/` 与 `client/dist/`）调整属主，保留权限模式且不使用 `1777`。如果 `/workspace` 本身是挂载点，则不改动宿主目录树；媒体、证书和其他挂载点会跳过。专用读写 `/workspace/db` 卷只处理该目录及 `DB_PATH` 指定的 SQLite 文件和 `-wal`、`-shm`、`-journal` 文件；只读数据库卷不改动。`DB_PATH` 不会扩大初始化范围，`/workspace` 外的路径不会被 chown。属主匹配后，后续启动只做 O(1) 元数据检查，不重复遍历或 chown。

初始化入口仅在启动阶段使用镜像的 `CAP_CHOWN` 文件能力；执行 Go 服务前会清空 capability 并设置 `no_new_privs`。如果运行配置启用了 `no-new-privileges` 或移除了 `CHOWN`，请预先将目录和卷设为匹配的 UID:GID。

`build.sh` 从脚本所在的当前 Git 工作区构建，不会拉取或切换分支；镜像会记录源码 Commit，工作区有未提交改动时标记为 `dirty`。部署前在 `$HOME/workspace/NAS-TikTok/.env` 创建 Docker `KEY=value` 格式的运行时配置文件，至少填入 `ACCOUNT`、`PASSWORD` 和不少于 32 字节的 `JWT_SECRET`。需要 HTTPS 时设置 `SSL=true`，并将 `fullchain.pem` 与 `privkey.pem` 放在同目录的 `certs/` 下；媒体放入 `static/`。根路径部署无需额外设置；子路径部署还需在运行时 `.env` 中设置 `APP_URL_PREFIX=photos`，并以 `VITE_BASE_URL=/photos/ bash ./build.sh` 构建匹配的前端。运行 `bash ./build.sh` 后选择构建、部署或构建并部署。部署会让你选择 `host` 或 `bridge` 网络和服务端口；若没有旧容器且系统提供 `ss`，脚本会先尽力检查 TCP 端口是否已被本机进程占用，Docker 启动阶段仍可能发现端口冲突。它会询问 `.env` 是否启用 HTTPS；若使用默认证书路径，会检查证书和私钥文件存在、非空且当前用户可读，自定义路径则由容器启动健康检查验证。媒体与证书目录以只读方式挂载。脚本先创建候选容器，启动后等待最多 90 秒的容器内 loopback 健康检查；只有健康后才移除旧容器，失败时会尝试恢复旧容器原先的运行状态。健康检查访问 `APP_URL_PREFIX` 下的 `/healthz`，HTTPS 探测会跳过本地证书信任校验。Docker 部署尚未验证。

`host` 网络下服务直接监听 NAS 端口；`bridge` 网络下脚本会发布所选端口。Bridge 网络通常只能看到 Docker 网桥一侧的来源地址；若需通过反向代理识别客户端 IP，请按实际拓扑配置可信代理。容器默认不限制来源 IP（`ALLOWED_IPS` 未设置或为空）；这不会关闭登录认证或其他安全校验。如需启用白名单，请在 `.env` 中设置逗号分隔的 IP 或 CIDR，并根据应用实际看到的来源地址配置。

如需在另一台 NAS 上离线启动，可在构建机上使用 `docker save` 导出镜像，再在 NAS 上用 `docker load` 导入；镜像构建及导入流程尚未验证。

## 功能与建议

如果有任何功能与建议，欢迎在 `Issues` 中提出

如果愿意对此项目做出贡献，欢迎提交 `PR`，非常感谢您的支持！

## 联系我

您可以联系我的邮箱 <a href="mailto:ASentimentalMan@GMail.com">ASentimentalMan@GMail.com</a>

## 许可协议

[GPLv3](LICENSE)
