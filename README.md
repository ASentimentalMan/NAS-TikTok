<h1 align="center">
  NAS-TikTok
</h1>

<p align="center">
 <a href="README.md">简体中文</a> | <a href="README_en.md">English</a>
</p>

<p align="center">
<a><img src="https://img.shields.io/badge/license-GPLv3-orange"/></a>
<a><img src="https://img.shields.io/badge/PRs-welcome-brightgreen"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-NestJS-blue"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-Vue.js-blue"/></a>

</p>

你是否因为 NAS 资源太多而不知该看哪个？现在 `NAS-TikTok` 来帮你解决这个烦恼了！`NAS-TikTok` 是一个前端模仿 `抖音` ，后端对资源库进行随机抽样的项目。让你拥有刷抖音一样浏览 NAS 资源的丝滑流畅体验！

## 技术栈

### 后端 NestJS

1. 负责 AUTH 鉴权

2. 对资源库进行水库抽样（无数据库/无缓存）

3. 支持 HTTPS HTTP2（需自备域名/证书）

4. 支持只允许局域网访问（docker 需要 host 模式）

5. 支持客户端 IP 绑定（即使 URL 泄露，IP 不一样也无法查看）

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

注意：本项目仅适用于学习和研究，不得用于商业使用

### 本地开发

---

```bash
git clone https://github.com/ASentimentalMan/NASTikTok
cd NASTikTok

# 创建媒体文件夹，把测试的资源放到里面
mkdir NASTikTok/statics

# 后端
cd NASTikTok/server
npm install
npm run dev

# 前端
cd NASTikTok/client
npm install
npm run dev
```

打开浏览器并访问: [http://127.0.0.1:3000](http://127.0.0.1:3000)

### 正式部署

---

#### Docker 内部结构

```bash
/workspace
├── certs/ # 证书文件
│   ├── fullchain.pem # 证书
│   └── privkey.pem # 私钥
├── static/ # 资源目录
├── server/
│   └── ...
└── client/
    └── ...
```

#### 在线拉取

```bash
# 拉取docker镜像
docker pull asentimentalman/nas-tiktok:latest
```

#### 本地构建

```bash
# 创建docker镜像
docker build -t nas-tiktok .

# 如果NAS不能运行dockerfile，请电脑build导出后再导入NAS进行启动
docker save -o nas-tiktok.tar nas-tiktok
```

#### Docker 启动命令

```bash
# 启动参数说明
docker run -d \
  --name nas-tiktok \
  # /certs -> 证书文件夹：需包含有效的 fullchain.pem 和 privkey.pem
  -v /certs:/workspace/certs \
  # /source -> 资源文件夹：需要被浏览的资源映射到这里
  -v /source:/workspace/statics \
  # port -> 映射端口：bridge模式才需要
  # bridge模式下docker无法拿到真实IP，更换IP退出与只能局域网访问功能将不可用
  -p port:port \
  # SSL -> 是否开启：如果开启的话必须提供certs文件夹及证书
  -e SSL=false \
  # LAN_ONLY -> 是否只允许局域网访问
  -e LAN_ONLY=false \
    # IPv6 -> 是否开启IPv6：开启的话LAN_ONLY会失效
  -e IPv6=false \
  # VITE_API_URL -> 访问地址：服务器/NAS的地址，例如192.168.1.100或者你的域名
  -e VITE_API_URL= \
  # APP_PORT -> 运行端口：例如3000（bridge模式时请与映射端口一致）
  -e VITE_API_PORT= \
  # ACCOUNT -> 你的账号
  -e ACCOUNT= \
  # PASSWORD -> 你的密码
  -e PASSWORD= \
  # JWT_SECRET -> TOKEN加密密钥：用来给token加密的字符串
  -e JWT_SECRET= \
  # JWT_SECRET -> TOKEN过期时间：默认1200秒（20分钟）
  -e JWT_EXPIRE=1200 \
  asentimentalman/nas-tiktok:latest
```

#### 补充说明

1. 本项目拥有 IP 变化下线和只允许局域网访问功能，建议跑在 docker 的--network host 模式，因为该模式才能拿到客户端 IP，否则这 2 项功能将失效

2. SSL：如果你有域名的话强烈建议开启！这将加密你的数据流量，起到保护隐私作用。如果没有域名，建议限制为只允许局域网访问

3. LAN_ONLY：只在 IPv4 环境起作用，且 `--network host` 时生效，因为只有绑定宿主机，dokcer 才能拿到真实的访问 IP，否则无法做出限制

4. IPv6：如果拥有 IPv6 环境并开启 IPv6 监听，就需要关闭 `LAN_ONLY`，因为无通过 IPv6 地址法判断是不是局域网访问，请求会全部拦截，导致无法访问

## 功能与建议

如果有任何功能与建议，欢迎在 `Issues` 中提出

如果愿意对此项目做出贡献，欢迎提交 `PR`，非常感谢您的支持！

## 联系我

您可以联系我的邮箱 <a href="mailto:ASentimentalMan@GMail.com">ASentimentalMan@GMail.com</a>

## 许可协议

[GPLv3](LICENSE)
