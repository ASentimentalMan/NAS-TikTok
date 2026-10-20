# NAS-TikTok Go Remaster 执行报告

## 1. 基线与范围

- 仓库：NAS-TikTok，工作分支 codex。
- 起始 HEAD：141ddc7cb6bc203be691ad0f798047ce412673da；本次没有创建 Commit，当前改动仍在工作区。
- 范围：Go 后端迁移、必要的前端认证/API适配、部署文件与说明、前端依赖升级、自动化质量门禁和本地验证。
- 环境边界：未运行 Docker；未向远端传输媒体。此前按要求对本地媒体目录做过读取验证；补充闭环使用 /tmp 内合成媒体。
- 交付判断：按用户本次限定范围，Go 服务已通过本地构建、测试和运行闭环，前端通过依赖检查、测试、类型检查与构建。Docker/NAS 和真实浏览器验证未执行；任务书中要求实测 Docker 的验收项仍为 NOT_RUN，不代表本地运行目标未完成。

## 2. M01–M14 模块映射

| 模块 | 当前实现 | 结果与限制 |
|---|---|---|
| M01 Runtime | server_go/cmd/nas-tiktok/main.go、internal/app/app.go | 单 Go 入口，支持 HTTP/HTTPS、信号处理和优雅关闭；本地启动闭环已验证。 |
| M02 Configuration | internal/config | 类型化配置、启动校验、旧环境变量兼容及 server_go/.env.example；空 ALLOWED_IPS 在本地配置下表示不限制来源，部署应明确设置。 |
| M03 HTTP | internal/httpapi | 路由、统一错误响应、方法/CORS处理、请求体限制、会话与访问策略衔接；含 /healthz、/api/session、/api/logout。 |
| M04 Network access | internal/access | IPv4/IPv6/CIDR、回环和可信代理来源处理；单测通过，Docker 网络拓扑未实测。 |
| M05 Captcha | internal/captcha | 数学 SVG、TTL、单次消费、容量控制和密码学随机数；单测通过。 |
| M06 Authentication | internal/auth、internal/httpapi | 单账号环境凭据校验、登录失败处理与限速；未引入数据库或多用户模型。 |
| M07 Session | internal/session、前端 API/router | HttpOnly Cookie、过期和撤销；前端不再把长期 JWT 加入媒体 URL。跨站和真实 Safari 行为未实测。 |
| M08 Media storage | internal/media/catalog、internal/media/streaming | 媒体根目录约束、扩展名与 MIME、只读使用方式；目录外访问和符号链接测试通过。 |
| M09 Discovery/sampling | internal/media/catalog、internal/media/sampling | 递归扫描、跳过 @eaDir、有限样本水库抽样；无大 NAS 性能基准。 |
| M10 Streaming | internal/media/streaming | 使用标准库文件流，支持 GET、HEAD、Range、206/416 和私有 no-store 缓存策略；自动化覆盖了主要范围，故障注入不完整。 |
| M11 Static web | internal/web、client/dist、client/public/env.js | Go 同源托管静态产物、SPA 回退和运行时配置；本地服务闭环通过。 |
| M12 Logging/errors | internal/app、internal/httpapi | 使用标准库 logger，避免记录密码、Cookie 和媒体路径；未另建 observability 包。 |
| M13 Shared utilities | access/auth/captcha/session/media 各领域包 | 领域工具留在使用模块中，没有引入集中式通用工具包。 |
| M14 Build/deployment | Dockerfile、.dockerignore、build.sh、.github/workflows/ci.yml、README.md、README_en.md | 生产配方切为 Go 单进程并复制 Vue dist；脚本实现 host/bridge、只读挂载、健康检查、健康失败回滚和基础预检。Docker、架构构建和真实部署回滚未验证。 |

## 3. API 与配置契约

| 路由 | 当前行为 |
|---|---|
| GET /api/captcha | 返回验证码 id 与 SVG Data URI。 |
| POST /api/logIn | 仍接收 account、password、codeId、code；成功时设置 HttpOnly 会话 Cookie，返回 {"ok":true}，不再向 JavaScript 返回长期 JWT。 |
| GET /api/list?type=image\|video | 需要有效会话，返回最多 10 个媒体 URL 的 data 数组。 |
| GET/HEAD /api/stream/... | 需要有效会话；支持标准 Range 和媒体流响应。 |
| POST /api/logout、GET /api/session、GET /healthz | 新增会话登出/检查与健康检查路由。 |

新配置区分 LISTEN_HOST/LISTEN_PORT、API_BASE、APP_URL_PREFIX、MEDIA_ROOT、STATIC_ROOT、TLS_CERT_FILE/TLS_KEY_FILE，并支持 ACCOUNT、PASSWORD、JWT_SECRET、JWT_EXPIRE、ALLOWED_IPS、ALLOW_IPv6、TRUSTED_PROXIES、CORS_ORIGINS、COOKIE_SECURE、COOKIE_SAME_SITE 和限流/容量设置。旧 VITE_API_URL/VITE_API_PORT 仍用于兼容解析。真实部署凭据由运行时提供；报告未复制或读取真实密钥。

## 4. 变更文件概览

- 新增 server_go/：Go 单入口、配置、访问控制、鉴权、验证码、会话、HTTP API、媒体扫描/抽样/流、静态托管及相应测试。
- 修改 client/：Cookie认证、运行时 API 基址、媒体列表/播放器错误路径及 Vue 类型声明生成结果；升级 package.json 和 package-lock.json。
- 修改 Dockerfile、.dockerignore、build.sh、README.md、README_en.md；新增 .github/workflows/ci.yml。
- 新增本报告。未修改原始 function.md、audit.md 和 todo.md。

### 前端直接依赖版本

20 个原直接依赖及 3 个测试依赖已更新到 registry 最新稳定兼容版本；最终 `npm outdated --json` 为空，锁文件与直接依赖安装树一致。TypeScript 使用 npm:@typescript/typescript6@^6.0.2（锁定 6.0.2）：当前最新 vue-tsc 3.3.13 仍解析 typescript/lib/tsc，而 TypeScript 7.0.2 不导出该子路径。TypeScript 7 下类型检查/完整构建失败；vue-tsc 的官方兼容别名通过类型检查与构建。当前锁定组合的 npm audit 为 0 vulnerabilities。

- Vue 3.5.44、Vue Router 5.4.0、Pinia 4.1.0、Swiper 14.3.0、@vueuse/core 15.0.0。
- Vite 8.3.4、@vitejs/plugin-vue 6.0.10、@vitejs/plugin-vue-jsx 5.1.6、vue-tsc 3.3.13。
- @iconify/vue 5.0.4、pinia-plugin-persistedstate 4.7.1、@tsconfig/node22 22.0.6、@types/node 26.6.5、@vue/tsconfig 0.9.1、less 4.9.1、npm-run-all2 9.0.3、unplugin-auto-import 21.1.0、unplugin-vue-components 32.1.0、vite-plugin-vue-devtools 8.2.1。
- 测试依赖：Vitest 5.0.5、@vue/test-utils 2.5.1、jsdom 30.1.3。

npm-run-all2 9.0.3 要求 Node ^22.22.2、^24.15.0 或 >=26，npm >=10。本机 Node 24.22.0 / npm 11.19.1 满足要求。Docker 前端构建与 CI 均固定使用 Node 22.23.3；Docker 官方清单列出 `node:22.23.3-alpine3.24` 并支持 amd64、arm64。镜像仍未固定 digest，也未执行 Docker 构建。

## 5. 审计追踪 AUDIT-001–036

状态含义：FIXED 表示实现已处理该发现，证据列说明了实际测试；FIXED（静态）表示仅从代码审查确认；PARTIAL 表示仍有明确缺口；OWNER_ACTION/OWNER_DECISION 表示需要所有者处理；NOT_RUN 表示环境验证未执行。

| ID | 状态 | 证据与残留 |
|---|---|---|
| AUDIT-001 | FIXED | catalog 使用根目录约束和符号链接拒绝；目录穿越、编码路径测试通过。并发替换路径竞态未专门验证。 |
| AUDIT-002 | OWNER_ACTION / 未核 | 镜像上下文排除 .env*；未检查 Git 历史或外部部署，无法确认历史凭据是否清理/轮换。所有者应评估曾公开的凭据并轮换。 |
| AUDIT-003 | PARTIAL | 配置要求 JWT_SECRET 至少 32 字节；长度不能保证密钥熵或不可预测性。 |
| AUDIT-004 | FIXED | IPv4/IPv6 CIDR、代理信任边界和回环逻辑有单测；真实 Docker/反向代理拓扑未测。 |
| AUDIT-005 | FIXED | 浏览器会话改用 HttpOnly Cookie，媒体 URL 不附加 Token；本地端到端闭环通过。Safari/跨站实机行为未测。 |
| AUDIT-006 | FIXED | 私有媒体使用 private, no-store；媒体响应头测试通过。 |
| AUDIT-007 | MITIGATED_BY_REPLACEMENT | 生产运行路径为单 Go 进程，验证码使用有界进程内存；多副本间验证码/会话/限流不共享。 |
| AUDIT-008 | FIXED | CORS/预检按来源与方法处理；授权和拒绝测试通过。 |
| AUDIT-009 | FIXED（静态） | 前端仅记住账号并清理旧 Token；旧 savedLogin 被重写为仅账号。无浏览器自动化覆盖。 |
| AUDIT-010 | PARTIAL | 由 net/http.ServeContent 处理 Range；测试覆盖开区间、定长、后缀、超大后缀、空文件、反向/畸形/不支持单元、多范围忽略、HEAD Range 和 416；客户端断连仍未覆盖。 |
| AUDIT-011 | PARTIAL | 文件流使用 os.File 和 defer Close；无 NAS 断开、权限改变或读取中删除故障注入。 |
| AUDIT-012 | PARTIAL | 验证码/IP、登录/IP、账号失败限速及有界 LRU 已实现；多副本状态不共享。 |
| AUDIT-013 | PARTIAL | Dockerfile 提供 loopback 健康检查；build.sh 创建候选容器，健康后才清理旧容器，检测到启动/健康失败时尝试恢复旧容器，并检查端口范围及默认证书文件。Docker 未运行；脚本中断时无自动恢复 trap，宿主机用户可读不保证容器 `app` 用户可读，自定义证书路径没有静态预检。 |
| AUDIT-014 | PARTIAL | Dockerfile 最终层只复制 dist 与 Go 二进制，.dockerignore 排除 .env*；未检查镜像层或运行密钥扫描。 |
| AUDIT-015 | FIXED（静态） | Session claims 不再绑定旧 HTTP Header；未有专门的旧 Header 绑定回归用例。 |
| AUDIT-016 | PARTIAL | 会话支持到期、容量上限和登出撤销；进程内状态不跨副本，未提供改密/密钥轮换会话流程。 |
| AUDIT-017 | FIXED（静态） | 验证码与会话 ID 使用 crypto/rand；未对随机数生成器做独立统计测试。 |
| AUDIT-018 | PARTIAL | 配置对端口、布尔值、TTL、CIDR、Origin 和必填项校验；本地 ALLOWED_IPS 为空时放行所有来源，部署应明确配置。 |
| AUDIT-019 | FIXED（静态） | HTTP 日志记录方法、状态和耗时，不记录凭据或媒体路径；错误响应使用通用消息。 |
| AUDIT-020 | PARTIAL | 扫描并发受控、可取消并按批读取，抽样内存有界；仍须遍历整棵目录树，没有大型 NAS 性能基准或时限。 |
| AUDIT-021 | FIXED | 前端媒体以 URL 为稳定 ID，列表去重并限制数量；新增图片/视频组件测试覆盖跨批次去重。 |
| AUDIT-022 | FIXED | 列表请求有进行中标记、AbortController、卸载取消和退避；新增组件测试覆盖并发锁定、卸载取消及图片/视频列表去重。 |
| AUDIT-023 | FIXED（静态） | 随机播放等待媒体可 seek/metadata；Safari 和真实浏览器播放未测。 |
| AUDIT-024 | FIXED（静态） | 播放器显式接收并检查事件 target。 |
| AUDIT-025 | FIXED（静态） | 页面、播放器和音乐组件实现监听器、计时器清理；无内存泄漏实测。 |
| AUDIT-026 | PARTIAL | 图片/视频处理 401、403、404、416 与网络失败；API 测试覆盖 401 与网络失败处理，但无播放器媒体错误事件/浏览器自动化覆盖。 |
| AUDIT-027 | FIXED（静态） | 首页和 Swiper 初始标签索引一致。 |
| AUDIT-028 | FIXED（静态） | 相关表达式增加长度比较和空值保护；没有独立行为测试。 |
| AUDIT-029 | FIXED（静态） | 应用启动等待 Router 初始化完成；无浏览器测试。 |
| AUDIT-030 | FIXED（当前 npm 快照） | Vite 升级到 8.3.4；本地 npm audit 实跑为 0 vulnerabilities。CI/镜像扫描未运行。 |
| AUDIT-031 | PARTIAL | Go 配置、认证、验证码、会话、HTTP、媒体目录/抽样/流及前端 API、登录持久化、媒体列表生命周期、Toast 清理测试通过；仍无真实浏览器/Safari/播放器测试，后端故障注入矩阵也非全覆盖。 |
| AUDIT-032 | PARTIAL | CI workflow 定义 Go test/race/vet/build、前端 npm ci/test/type-check/build/audit；workflow 未运行，没有 Docker build、秘密扫描或分支保护证据。 |
| AUDIT-033 | PARTIAL | npm lock/ci、精确 Node 22.23.3 前端构建标签和 Go `-trimpath` 提高可追溯性；基础镜像未固定 digest，未执行镜像扫描或 Docker 构建。 |
| AUDIT-034 | PARTIAL | build.sh 从当前工作区构建并写入 HEAD/dirty 标签；不校验分支、不指定 Commit，也不拒绝 dirty 工作区。 |
| AUDIT-035 | PARTIAL | README 已改为 Go 运行方式并描述 host/bridge、端口、只读挂载、TLS 及健康回滚；Docker 启动、TLS、NAS 操作未按文档实际验证。 |
| AUDIT-036 | OWNER_DECISION | LICENSE 为 GPLv3，README 中英文禁止商业用途，遗留 server/package.json 标为 UNLICENSED；本次未改授权文本，需所有者/法律决策。 |

## 6. VERIFY-001–012

| ID | 状态 | 结果 |
|---|---|---|
| VERIFY-001 | MITIGATED_BY_REPLACEMENT | Nest/Fastify 通配路由已由 Go 路由替代；编码多级路径集成测试随 Go 测试通过。 |
| VERIFY-002 | VERIFIED（本地测试） | 会话在媒体流前验证；Cookie GET/Range 与匿名 GET/HEAD Range 均有集成测试，未授权响应不返回媒体内容。 |
| VERIFY-003 | MITIGATED_BY_REPLACEMENT / NOT_RUN | 不再比较请求 Header；TLS/HTTP2 浏览器组合未测。 |
| VERIFY-004 | MITIGATED_BY_REPLACEMENT / NOT_RUN | 运行入口改为 Go 二进制；Docker 启动未执行。 |
| VERIFY-005 | PARTIAL / NOT_RUN | 最终配方只复制构建产物且忽略 .env*；未扫描实际镜像层。 |
| VERIFY-006 | PARTIAL / NOT_RUN | 代理 Header 信任边界有单测；host/bridge/代理网络未实测。 |
| VERIFY-007 | NOT_RUN | 未使用 Safari/iOS 真机或浏览器验证播放器与 Range。 |
| VERIFY-008 | PARTIAL / NOT_RUN | 扩展名/MIME 映射在代码中；目标浏览器编解码能力未测。 |
| VERIFY-009 | NOT_RUN | 无大型 NAS 目录性能、CPU、内存或 I/O 数据。 |
| VERIFY-010 | NOT_RUN | 有卸载清理逻辑，未进行频繁切换或内存测量。 |
| VERIFY-011 | NOT_RUN | 未进行断网、磁盘断开、权限变化或读取中删除故障注入。 |
| VERIFY-012 | VERIFIED（npm）/其余未验证 | 当前完整 npm audit 为 0 vulnerabilities；不代表 Docker/CI/其他扫描器已执行。 |

## 7. TEST-001–009

| ID | 状态 | 自动化覆盖与缺口 |
|---|---|---|
| TEST-001 | PARTIAL | auth/captcha/session/httpapi 测试覆盖账号密码要求、验证码错误/过期/并发单次消费、登录限流、Cookie 会话、篡改/过期/撤销及缺少验证码；仍非全部配置/错误故障矩阵。 |
| TEST-002 | MITIGATED_BY_REPLACEMENT | 单 Go 进程替代 PM2 多进程；没有跨实例共享测试，多副本不受支持。 |
| TEST-003 | PARTIAL | access 和 httpapi 测试覆盖 CIDR、IPv4/IPv6、代理伪造；实际容器网络未覆盖。 |
| TEST-004 | PARTIAL | catalog/httpapi 测试覆盖根目录、符号链接、编码穿越和抽样；未覆盖证书/环境文件路径及 NAS 权限/删除故障。 |
| TEST-005 | PARTIAL | streaming 测试覆盖开区间、定长、后缀、多范围、畸形/反向范围、空文件、HEAD Range 和 416；客户端中断连接未覆盖。 |
| TEST-006 | PARTIAL | 前端测试覆盖 Cookie API、Session 状态、401 清理旧 Token/跳转、登录失败不保存密码及旧 `savedLogin` 密码清理；路由守卫与真实浏览器登录流程未测。 |
| TEST-007 | NOT_IMPLEMENTED | 未发现播放器组件或浏览器自动化测试。 |
| TEST-008 | PARTIAL | catalog 测试覆盖递归、过滤、扩展名、数量上限和批次；前端测试覆盖图片/视频列表去重、并发锁定和卸载取消，失败重试与真实浏览器仍未覆盖。 |
| TEST-009 | NOT_RUN | 未执行 Docker 构建、镜像检查、TLS、网络模式或 NAS 挂载验证。 |

## 8. 实际验证记录

以下均为本地执行；Go 缓存和构建产物定向到 /tmp。

| 命令/操作 | 结果 |
|---|---|
| Go 测试 | 通过；10 个有测试的 Go 包通过，cmd 和 app 无测试文件。 |
| Go 静态检查 | go vet 通过；gofmt 检查无输出。 |
| Go 构建 | 通过。 |
| 前端 type-check/build | 均通过；build 包含 type-check 与 Vite 8.3.4 production build。 |
| 前端测试 | 通过；4 个测试文件、12 项测试通过。 |
| npm audit / npm ls --depth=0 | 通过，0 vulnerabilities；依赖与锁文件版本一致。 |
| npm outdated --json | 返回空对象；未发现可兼容的过期直接依赖。 |
| bash -n build.sh / git diff --check | 通过。 |
| 本地服务闭环 | 使用 /tmp 合成媒体验证 /healthz、静态首页、验证码、登录、图片列表、HEAD、Range、登出及登出后 401；通过。 |

实际执行命令：

Go 命令从 server_go/ 执行；前端命令从 client/ 执行；npm 缓存和日志写入 /tmp。

    env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOPATH=/tmp/nas-tiktok-go-path GOMODCACHE=/tmp/nas-tiktok-go-modcache GOCACHE=/tmp/nas-tiktok-go-cache GOTMPDIR=/tmp go test -count=1 -mod=readonly ./...
    env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOPATH=/tmp/nas-tiktok-go-path GOMODCACHE=/tmp/nas-tiktok-go-modcache GOCACHE=/tmp/nas-tiktok-go-cache GOTMPDIR=/tmp go vet -mod=readonly ./...
    env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOPATH=/tmp/nas-tiktok-go-path GOMODCACHE=/tmp/nas-tiktok-go-modcache GOCACHE=/tmp/nas-tiktok-go-cache GOTMPDIR=/tmp go build -mod=readonly -o /tmp/nas-tiktok-go-bin/nas-tiktok ./cmd/nas-tiktok
    find . -type f -name '*.go' -print0 | xargs -0 gofmt -l
    bash -n build.sh
    git diff --check
    env npm_config_cache=/tmp/nas-tiktok-npm-cache npm_config_logs_dir=/tmp/nas-tiktok-npm-logs npm run type-check
    env npm_config_cache=/tmp/nas-tiktok-npm-cache npm_config_logs_dir=/tmp/nas-tiktok-npm-logs npm test
    env npm_config_cache=/tmp/nas-tiktok-npm-cache npm_config_logs_dir=/tmp/nas-tiktok-npm-logs npm run build
    npm --cache=/tmp/nas-tiktok-npm-cache --logs-dir=/tmp/nas-tiktok-npm-logs audit
    npm --cache=/tmp/nas-tiktok-npm-cache --logs-dir=/tmp/nas-tiktok-npm-logs ls --depth=0
    npm --cache=/tmp/nas-tiktok-npm-cache --logs-dir=/tmp/nas-tiktok-npm-logs ci --ignore-scripts

未运行 go test -race：本地环境 CGO_ENABLED=0 且没有 gcc；CI workflow 定义了 race 检查，但 CI 未运行。未运行 Docker、GitHub Actions、跨架构构建、浏览器/Safari 测试和 NAS 故障注入。用户指定媒体目录仅在本地读取验证，没有向远端传输。

## 9. 兼容性、人工操作与剩余风险

- HTTP 本地服务已验证；HTTPS 配置和 TLS 证书读取仅有代码/配置校验，未进行证书运行测试。Go 标准库支持 HTTP/2，但本次未用浏览器实测。
- IPv4/IPv6、CIDR、可信代理逻辑有单测；真实 host/bridge/反代拓扑仍需部署环境验证。空 ALLOWED_IPS 会放行所有来源，生产部署应明确设置白名单。
- 媒体路径边界、编码路径、符号链接和 Range 已有本地测试；未对大型目录、NAS 断连、权限变化和实际 iOS 播放进行故障/兼容性验证。
- Node 22.23.3 已满足 npm-run-all2 的最低版本要求，Docker 构建与 CI 配置已对齐。实际 Docker 构建/部署前仍需检查镜像内容，并验证启动健康检查、host/bridge、只读挂载、TLS 和失败回滚。
- 所有者应确认 AUDIT-002 历史凭据是否曾公开并自行轮换；本次未检查或更改外部部署凭据。
- 所有者/法律顾问需决定 AUDIT-036 实际授权意图，再统一 GPLv3、README 商业用途声明和遗留 server/package.json 元数据。
- 部署脚本的正常启动/健康失败分支已有恢复逻辑，但 Docker 未运行；脚本收到中断信号时没有自动恢复，默认 TLS 文件可读预检按宿主机当前用户判断而非容器 `app` 用户。Dockerfile 基础镜像未固定 digest，部署脚本未校验分支/干净状态。以上仍是部署风险。
