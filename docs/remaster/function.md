
NAS-TikTok 后端功能事实清单
Backend Functional Inventory
========================================

一、源码基准
========================================

仓库：
ASentimentalMan/NAS-TikTok

分支：
dev

Commit：
141ddc7cb6bc203be691ad0f798047ce412673da

核对日期：
2026-10-19

技术栈：
- Node.js >= 22
- TypeScript
- NestJS 11
- Fastify 5
- @nestjs/jwt
- @nestjs/config
- @nestjs/cache-manager
- @nestjs/serve-static
- class-validator
- class-transformer
- svg-captcha
- mime-types
- PM2
- Docker

现有正式业务模块：

1. AuthModule
2. StreamModule

其他功能分布在：

- main.ts
- app.module.ts
- configs/
- guards/
- decorators/
- filters/
- types/
- utils/
- Dockerfile
- entrypoint.sh
- ecosystem.config.js
- build.sh

本清单按照这些源码已经承担的职责，
将后端归类为 14 个功能领域。

分类并不改变现有代码事实。


二、模块总览
========================================

M01 - Application Runtime
      应用启动与运行时

M02 - Configuration
      应用配置与环境变量

M03 - HTTP Infrastructure
      HTTP 服务与全局请求处理

M04 - Network Access Control
      IP 地址访问控制

M05 - Captcha
      图形验证码

M06 - Account Authentication
      账户身份验证

M07 - JWT & Session
      令牌签发与身份校验

M08 - Media Storage
      媒体存储位置与文件类型

M09 - Media Discovery & Sampling
      媒体目录扫描与随机抽样

M10 - Media Streaming
      媒体文件传输与 Range 请求

M11 - Static Web Hosting
      前端静态资源托管

M12 - Error Handling & Logging
      异常处理与日志

M13 - Common Types & Utilities
      公共类型与工具函数

M14 - Build & Deployment
      构建、容器与进程运行

其中：

现有 AuthModule 覆盖 M05、M06、M07 的主要业务。

现有 StreamModule 覆盖 M08、M09、M10 的主要业务。

其余领域目前由框架级配置、全局组件、
工具函数或部署文件实现。


========================================
M01 - APPLICATION RUNTIME
应用启动与运行时
========================================

源文件：

server/src/main.ts
server/src/app.module.ts

----------------------------------------
M01.01 应用入口
----------------------------------------

入口函数：

bootstrap()

主要行为：

1. 创建 FastifyAdapter。
2. 创建 NestFastifyApplication。
3. 加载 AppModule。
4. 获取服务端配置。
5. 配置 CORS。
6. 设置全局 API 前缀。
7. 注册全局异常过滤器。
8. 注册全局请求校验管道。
9. 注册进程级异常事件。
10. 启动 HTTP 服务。

入口最终调用：

bootstrap()

----------------------------------------
M01.02 HTTP / HTTPS 启动
----------------------------------------

启动模式取决于：

process.env.SSL

当 SSL === "true"：

读取：

/workspace/certs/privkey.pem
/workspace/certs/fullchain.pem

设置 Fastify：

https.key
https.cert
http2 = true

其他情况：

不向 FastifyAdapter 传入上述 TLS 配置。

----------------------------------------
M01.03 服务监听
----------------------------------------

监听参数：

appConf.port
appConf.host

来源：

ConfigService.get("server")

实际调用：

app.listen(port, host, callback)

监听地址由 ALLOW_IPv6 决定：

false：
0.0.0.0

true：
::

----------------------------------------
M01.04 启动日志
----------------------------------------

启动回调中检查：

- process.env.NODE_APP_INSTANCE
- cluster.isPrimary

符合主进程判断时输出：

应用名称
访问地址
端口
URL 前缀

日志格式：

APP_NAME is running on
APP_URL:PORT/URL_PREFIX

----------------------------------------
M01.05 进程级异常监听
----------------------------------------

注册：

process.on("uncaughtException")

处理：

console.error(error)

注册：

process.on("unhandledRejection")

处理：

console.warn(reason)

----------------------------------------
M01.06 根模块组装
----------------------------------------

AppModule 注册：

ConfigModule
CacheModule
ServeStaticModule
AuthModule
StreamModule

同时注册两个全局 Guard Provider：

JWTProvider
IPProvider

AppModule 自身没有额外声明业务 Controller。


========================================
M02 - CONFIGURATION
应用配置与环境变量
========================================

源文件：

server/src/configs/server.config.ts
server/src/configs/account.config.ts
server/src/configs/jwt.config.ts
server/src/app.module.ts

----------------------------------------
M02.01 环境文件加载
----------------------------------------

使用：

ConfigModule.forRoot()

配置：

isGlobal: true

envFilePath：

1. .env.local
2. .env.${NODE_ENV}
3. .env

注册三个命名配置：

server
account
jwt

----------------------------------------
M02.02 ServerConfig
----------------------------------------

命名空间：

server

字段：

appName
appURL
urlPrefix
host
port
staticURL
allowIPv6
allowedIPs

对应环境变量：

APP_NAME
VITE_API_URL
APP_URL_PREFIX
ALLOW_IPv6
VITE_API_PORT
ALLOWED_IPS

字段转换：

appName：
String(APP_NAME)

appURL：
String(VITE_API_URL)

urlPrefix：
String(APP_URL_PREFIX)

host：
ALLOW_IPv6 === "true" ? "::" : "0.0.0.0"

port：
Number(VITE_API_PORT)

allowIPv6：
ALLOW_IPv6 === "true"

allowedIPs：
String(ALLOWED_IPS)

staticURL：

VITE_API_URL
+
":"
+
VITE_API_PORT
+
APP_URL_PREFIX
+
"/stream"

staticURL 用于生成媒体列表中的访问 URL。

----------------------------------------
M02.03 AccountConfig
----------------------------------------

命名空间：

account

字段：

account
password

来源：

ACCOUNT
PASSWORD

两个字段都转换为字符串。

该配置在登录认证时读取。

----------------------------------------
M02.04 JWTConfig
----------------------------------------

命名空间：

jwt

字段：

accessSecret
accessExpiresIn

来源：

JWT_SECRET
JWT_EXPIRE

转换：

JWT_SECRET -> String
JWT_EXPIRE -> Number

JWT 配置被 AuthModule 的 JwtModule 使用。

----------------------------------------
M02.05 运行环境文件
----------------------------------------

当前仓库包含：

server/.env
server/.env.development
server/.env.production

.env 包含基础应用配置。

.env.development 包含开发环境下的：

- API 地址和端口
- 账号
- 密码
- IP 相关参数
- SSL 参数
- JWT 参数

.env.production 包含对应的生产配置字段。

此处仅列出配置键，不展示配置值。


========================================
M03 - HTTP INFRASTRUCTURE
HTTP 服务与全局请求处理
========================================

源文件：

server/src/main.ts
server/src/app.module.ts

----------------------------------------
M03.01 HTTP 框架
----------------------------------------

NestFactory 使用：

FastifyAdapter

HTTP 路由由 NestJS Controller 注册。

----------------------------------------
M03.02 全局 API 前缀
----------------------------------------

代码：

app.setGlobalPrefix("api")

业务 API 使用：

/api/...

当前 Controller 没有额外的类级路径前缀。

----------------------------------------
M03.03 CORS
----------------------------------------

调用：

app.enableCors()

配置：

origin: "*"

credentials: true

methods:
- GET
- OPTIONS

----------------------------------------
M03.04 全局请求校验
----------------------------------------

使用：

ValidationPipe

设置：

stopAtFirstError: true
whitelist: true
forbidUnknownValues: true
transform: true

对应能力：

- 请求 DTO 校验
- 基于装饰器的字段保留
- 数据转换
- 检测未知验证对象
- 单字段遇到首个错误后停止继续收集

----------------------------------------
M03.05 全局请求守卫
----------------------------------------

通过 APP_GUARD 注册：

JWTGuard
IPGuard

二者适用于全局 HTTP 路由处理链。

JWTGuard 识别 @Public 元数据。

IPGuard 独立读取 IP 白名单配置。

----------------------------------------
M03.06 全局异常过滤器
----------------------------------------

注册：

ErrorExceptionsFilter
HttpExceptionFilter

统一由 NestJS 的异常过滤机制处理。


========================================
M04 - NETWORK ACCESS CONTROL
网络访问控制
========================================

源文件：

server/src/guards/ip.guard.ts
server/src/utils/tool.utils.ts
server/src/configs/server.config.ts

----------------------------------------
M04.01 全局 IP Guard
----------------------------------------

类：

IPGuard

注册方式：

APP_GUARD

入口：

canActivate(context)

读取：

server.allowedIPs
server.allowIPv6

获取请求 IP：

request.ip

----------------------------------------
M04.02 IP 检查触发条件
----------------------------------------

当 allowedIPs 为真值时：

调用：

isAllowedIPs(
  request.ip,
  allowedIPs,
  allowIPv6
)

返回 true：

允许当前请求继续。

返回 false：

记录日志并抛出：

ForbiddenException("禁止访问")

----------------------------------------
M04.03 IPv4 地址识别
----------------------------------------

使用 Node.js：

net.isIPv4()

将 IPv4 地址转换成 32 位无符号整数。

辅助函数：

ip4ToNum(ip)

计算方式：

解析四个八位组，
利用位运算合成 IPv4 整数。

----------------------------------------
M04.04 IPv4 白名单解析
----------------------------------------

ALLOWED_IPS 使用逗号分隔。

处理步骤：

1. split(",")
2. trim()
3. filter(Boolean)

每个配置项可以包含：

IPv4 地址
IPv4 CIDR 网段

未指定掩码时默认：

/32

使用掩码计算目标地址是否属于指定范围。

----------------------------------------
M04.05 内置本地地址
----------------------------------------

直接返回 true 的字符串：

localhost
127.0.0.1
::1

IPv4 地址检查还包含：

127.0.0.0/8

----------------------------------------
M04.06 IPv6 地址处理
----------------------------------------

使用：

net.isIPv6()

当前函数的 IPv6 分支：

当 allowIPv6 为 true，
并且目标地址是合法 IPv6 地址时，
直接返回 true。

IPv6 监听开关与访问检查都使用
ALLOW_IPv6 相关配置。

----------------------------------------
M04.07 IP 拒绝日志
----------------------------------------

Logger：

IPGuard

输出：

禁止访问 @ 请求IP

HTTP 异常：

403 Forbidden

----------------------------------------
M04.08 关联范围
----------------------------------------

IPGuard 不依赖用户是否登录。

标记为 @Public 的路由，
仍在 IPGuard 的全局处理范围内。


========================================
M05 - CAPTCHA
图形验证码
========================================

源文件：

server/src/modules/auth/code.service.ts
server/src/guards/code.guard.ts
server/src/modules/auth/auth.controller.ts

所在实际模块：

AuthModule

----------------------------------------
M05.01 验证码 API
----------------------------------------

HTTP：

GET /api/captcha

Controller 方法：

AuthController.captcha()

调用：

CodeService.captcha()

该接口具有：

@Public()

----------------------------------------
M05.02 验证码图像生成
----------------------------------------

使用依赖：

svg-captcha

具体调用：

svgCaptcha.createMathExpr()

生成数学表达式类型的验证码。

参数：

size: 4
ignoreChars: "0o1Il"
noise: 4
color: true
background: "transparent"
width: 100
height: 32
fontSize: 48

生成内容：

svg.data
svg.text

svg.data：
SVG 图像内容。

svg.text：
验证码期望答案。

----------------------------------------
M05.03 验证码 ID 生成
----------------------------------------

方法：

getUniqueUUID()

调用公共函数：

generateUUID()

然后检查缓存：

cacheManager.get(id)

如果缓存中已经存在相同 ID，
则重新生成。

如果不存在，
返回新的 ID。

----------------------------------------
M05.04 验证码缓存写入
----------------------------------------

缓存键：

codeId

缓存值：

svg.text

调用：

cacheManager.set(
  id,
  value,
  5 * 60 * 1000
)

TTL：

300000 毫秒
即 5 分钟。

缓存使用全局 CacheModule。

未配置外部缓存存储后端。

----------------------------------------
M05.05 验证码图像编码
----------------------------------------

使用：

Buffer.from(svg.data).toString("base64")

最终编码格式：

data:image/svg+xml;base64,...

----------------------------------------
M05.06 验证码 API 响应
----------------------------------------

响应 JSON：

{
  "id": "<验证码ID>",
  "data": "<SVG Data URI>"
}

id：

用于后续校验。

data：

用于前端显示验证码图像。

----------------------------------------
M05.07 验证码验证
----------------------------------------

方法：

CodeService.checkVerifyCode(id, code)

处理：

1. 通过 ID 读取缓存。
2. 如果缓存值存在，删除该缓存键。
3. 将输入 code 转为小写。
4. 将缓存答案转为小写。
5. 比较两个字符串。
6. 一致则正常结束。
7. 不一致抛出 BadRequestException。

异常消息：

验证码错误

----------------------------------------
M05.08 登录验证码 Guard
----------------------------------------

类：

CodeGuard

方法：

canActivate(context)

从 HTTP Body 读取：

codeId
code

如果任意字段缺失：

抛出：

BadRequestException("请输入验证码")

如果两个字段都存在：

调用：

CodeService.checkVerifyCode()

验证通过：

return true

----------------------------------------
M05.09 验证码与登录的关系
----------------------------------------

CodeGuard 被注册在：

POST /api/logIn

注册方式：

@UseGuards(CodeGuard)

账户密码校验位于后续 AuthService 中。

验证码与账号密码属于两个独立的处理步骤。


========================================
M06 - ACCOUNT AUTHENTICATION
账户身份验证
========================================

源文件：

server/src/modules/auth/auth.controller.ts
server/src/modules/auth/auth.service.ts
server/src/modules/auth/auth.dto.ts
server/src/configs/account.config.ts

所在实际模块：

AuthModule

----------------------------------------
M06.01 登录 API
----------------------------------------

HTTP：

POST /api/logIn

Controller：

AuthController.logIn()

装饰器：

@Public()
@UseGuards(CodeGuard)

----------------------------------------
M06.02 登录请求格式
----------------------------------------

请求体：

{
  "account": "string",
  "password": "string",
  "codeId": "string",
  "code": "string"
}

DTO：

AuthDto

继承：

VerifyCodeDto

----------------------------------------
M06.03 DTO 字段
----------------------------------------

VerifyCodeDto：

codeId?: string
code?: string

字段使用：

@IsOptional()
@Type(() => String)

AuthDto：

account: string
password: string

字段使用：

@IsNotEmpty()
@Type(() => String)

----------------------------------------
M06.04 登录请求上下文
----------------------------------------

Controller 同时接收：

@Body() dto
@Headers() headers
@Ip() ip

因此登录处理的主要输入为：

- 用户账号
- 用户密码
- 验证码
- 验证码 ID
- HTTP 请求头
- 客户端 IP

----------------------------------------
M06.05 账户配置读取
----------------------------------------

AuthService.logIn()

通过 ConfigService 获取：

account

其中包含：

account
password

现有账户来源是环境变量。

没有数据库账户查询流程。

----------------------------------------
M06.06 账号密码校验
----------------------------------------

首先检查配置是否存在：

user.account
user.password

如果不存在：

ForbiddenException("没有初始账户")

然后执行字符串直接比较：

account !== user.account

password !== user.password

任意一个不匹配：

BadRequestException("账号密码错误")

----------------------------------------
M06.07 登录成功处理
----------------------------------------

账户验证成功后：

调用：

TokenService.generateToken(headers, ip)

获取 JWT 字符串。

随后记录登录日志。

最终返回：

{
  "token": "<JWT>"
}

----------------------------------------
M06.08 登录日志
----------------------------------------

Logger：

AuthService

记录：

- 账号
- 请求 IP
- User-Agent

日志格式包含：

Logged In: account @ ip By user-agent

----------------------------------------
M06.09 现有账户模型
----------------------------------------

当前代码中的账户模型具有以下特征：

- 单组环境变量账号密码。
- 无数据库模型。
- 无用户 ID 查询。
- 无用户表。
- 无用户角色字段。
- 无用户注册 API。
- 无密码找回 API。
- 无账号管理 API。

这些是当前源码的实际边界。


========================================
M07 - JWT & SESSION
JWT 令牌与请求身份校验
========================================

源文件：

server/src/modules/auth/token.service.ts
server/src/modules/auth/auth.module.ts
server/src/guards/jwt.guard.ts
server/src/decorators/public.decorator.ts
server/src/decorators/jwt.decorator.ts
server/src/types/interfaces/auth.interface.ts

----------------------------------------
M07.01 JWT 服务注册
----------------------------------------

AuthModule 中使用：

JwtModule.registerAsync()

global: true

通过 ConfigService 获取：

jwt.accessSecret
jwt.accessExpiresIn

签名配置：

secret: accessSecret

signOptions.expiresIn:

`${accessExpiresIn}s`

JWT 服务作为全局模块注册。

----------------------------------------
M07.02 JWT 创建
----------------------------------------

类：

TokenService

方法：

generateToken(headers, ip)

功能：

生成 JWT Payload，
调用 JwtService.signAsync()，
返回 JWT 字符串。

----------------------------------------
M07.03 Session ID
----------------------------------------

每次调用 generateToken()：

执行：

generateUUID()

生成：

session

该字段写入 JWT Payload。

当前签发流程没有同时创建数据库会话记录。

----------------------------------------
M07.04 JWT Payload 字段
----------------------------------------

生成时使用：

host
connection
user-agent
dnt
sec-fetch-site
accept-language
session
ip

由 JWT 库签名时添加的时间相关声明包括：

iat
exp

类型声明 JWT 还包含：

accept
referer

但当前 generateToken() 没有主动赋值
accept 和 referer。

----------------------------------------
M07.05 JWT Guard
----------------------------------------

类：

JWTGuard

注册：

APP_GUARD

入口：

canActivate(context)

主要流程：

1. 检查路由是否标记 Public。
2. 判断令牌读取方式。
3. 提取令牌。
4. 校验 JWT 签名和有效期。
5. 比较请求头与 JWT 内的关联字段。
6. 将 JWT Payload 写入 request.jwt。
7. 允许请求继续。

----------------------------------------
M07.06 Public 路由识别
----------------------------------------

装饰器：

@Public()

实现：

SetMetadata("isPublic", true)

JWTGuard 使用：

Reflector.getAllAndOverride()

检查 Controller 方法及类上的元数据。

当 isPublic 为 true：

JWTGuard 直接返回 true。

----------------------------------------
M07.07 普通 API Token 提取
----------------------------------------

方法：

extractTokenFromHeader()

读取：

Authorization

期望格式：

Bearer <JWT>

代码将 Header 按空格拆分。

只有第一个字段等于 Bearer，
才返回第二个字段作为 Token。

----------------------------------------
M07.08 媒体请求 Token 提取
----------------------------------------

JWTGuard 判断：

request.url.startsWith(
  `${urlPrefix}/stream/`
)

当满足该条件：

读取：

request.query.auth

因此当前媒体请求使用：

/api/stream/xxx?auth=<JWT>

非媒体请求通常使用 Authorization Header。

----------------------------------------
M07.09 JWT 验证
----------------------------------------

调用：

jwtService.verifyAsync(token)

JWT 校验成功后得到 Payload。

如果缺少 Token：

UnauthorizedException("用户未登录")

如果 JWT 验证失败：

UnauthorizedException("登录已过期")

----------------------------------------
M07.10 请求上下文绑定
----------------------------------------

方法：

verifyHeaders(headers, jwt, ip)

比较：

host
connection
user-agent
dnt
sec-fetch-site
accept-language
ip

比较规则：

只有 JWT 自身拥有对应属性时，
才执行当前请求值与 JWT 值的字符串比较。

比较方式：

String(header) === String(jwtValue)

全部匹配：

返回 true。

任一字段不匹配：

返回 false。

----------------------------------------
M07.11 JWT 请求注入
----------------------------------------

验证成功后：

request.jwt = jwt

其他请求处理代码可以通过
request.jwt 获取验证后的 Payload。

----------------------------------------
M07.12 JWT 参数装饰器
----------------------------------------

定义：

@JWT()

源文件：

decorators/jwt.decorator.ts

功能：

从 request.jwt 读取 Payload。

当装饰器传入字段名称时：

返回 request.jwt[data]

没有传入字段名称时：

返回整个 request.jwt。

该装饰器已定义，
但当前现有业务 Controller 没有使用它。

----------------------------------------
M07.13 Session 相关现有行为
----------------------------------------

当前代码包含：

- JWT 签发
- JWT 过期时间
- Session UUID 生成
- 请求身份验证
- 请求 IP 关联
- 请求 Header 关联

当前后端源码没有单独的：

- Refresh Token API
- Logout API
- Token Revocation API
- Session List API
- Session Management Controller


========================================
M08 - MEDIA STORAGE
媒体存储与文件类型
========================================

源文件：

server/src/modules/stream/stream.service.ts

实际所在模块：

StreamModule

----------------------------------------
M08.01 媒体根目录
----------------------------------------

StreamService 构造函数设置：

workDir

开发环境：

join(
  __dirname,
  "../../..",
  "statics"
)

对应当前构建目录布局下的：

server/statics

其他环境：

/workspace/statics

----------------------------------------
M08.02 媒体存储方式
----------------------------------------

媒体文件来自文件系统目录。

使用 Node.js fs API：

- opendir
- existsSync
- statSync
- createReadStream

没有数据库媒体记录。

没有独立对象存储客户端。

----------------------------------------
M08.03 视频扩展名
----------------------------------------

VIDEO_EXTENSIONS：

.mp4
.webm
.mov
.ogv
.ogg

扩展名在扫描时转换为小写进行比较。

----------------------------------------
M08.04 图片扩展名
----------------------------------------

IMAGE_EXTENSIONS：

.jpg
.jpeg
.heic
.heif
.avif
.webp
.png
.gif
.apng
.bmp

扩展名在扫描时转换为小写进行比较。

----------------------------------------
M08.05 MIME 类型获取
----------------------------------------

方法：

getContentType(ext)

内置映射：

.jpg   -> image/jpeg
.jpeg  -> image/jpeg
.png   -> image/png
.gif   -> image/gif
.webp  -> image/webp
.bmp   -> image/bmp
.svg   -> image/svg+xml
.mp4   -> video/mp4
.webm  -> video/webm
.mov   -> video/quicktime

当内置映射不存在时：

调用：

mime-types.lookup(ext)

仍无法识别时：

application/octet-stream

该 MIME 映射用于文件传输，
与媒体列表使用的扩展名集合相互独立。

----------------------------------------
M08.06 文件属性
----------------------------------------

文件传输过程读取：

- 文件是否存在
- 文件大小
- 文件扩展名
- 文件名

文件大小来自：

statSync(path).size

文件名来自：

basename(path)

扩展名来自：

extname(path)

----------------------------------------
M08.07 当前媒体操作范围
----------------------------------------

现有后端提供：

- 文件查找
- 目录遍历
- 媒体抽样
- 文件读取
- 文件流传输

没有定义：

- 媒体上传 API
- 媒体删除 API
- 媒体重命名 API
- 媒体移动 API
- 文件写入 API
- 视频转码 API
- 缩略图生成 API


========================================
M09 - MEDIA DISCOVERY & SAMPLING
媒体发现、扫描和随机抽样
========================================

源文件：

server/src/modules/stream/stream.controller.ts
server/src/modules/stream/stream.service.ts

----------------------------------------
M09.01 媒体列表 API
----------------------------------------

HTTP：

GET /api/list

查询参数：

type=image
type=video

Controller：

StreamController.list()

调用：

StreamService.list(query)

该接口没有 @Public 标记，
使用全局 JWT 身份验证。

----------------------------------------
M09.02 媒体类型选择
----------------------------------------

方法：

list({ type })

内部逻辑：

type === "image"

选择：

IMAGE_EXTENSIONS

其他值：

选择 VIDEO_EXTENSIONS

TypeScript 参数类型声明为：

"image" | "video"

----------------------------------------
M09.03 每次抽样数量
----------------------------------------

固定变量：

pageSize = 10

单次列表响应最多包含：

10 个媒体 URL。

媒体数量不足 10 时，
返回实际选中的数量。

----------------------------------------
M09.04 目录遍历
----------------------------------------

内部异步函数：

traverse(dir)

从 workDir 开始。

使用：

fs/promises.opendir()

每次读取：

dirHandle.read()

处理目录中的每个 Dirent。

----------------------------------------
M09.05 子目录递归
----------------------------------------

遇到目录：

dirent.isDirectory()

递归调用：

traverse(fullPath)

递归为异步顺序遍历。

----------------------------------------
M09.06 Synology 特殊目录
----------------------------------------

当目录名称为：

@eaDir

跳过整个目录。

该名称在 NAS 场景中通常用于
Synology 生成的附加数据目录。

当前代码只对这一特定名称
进行显式目录跳过。

----------------------------------------
M09.07 文件筛选
----------------------------------------

遇到普通文件：

dirent.isFile()

取得扩展名：

extname(dirent.name).toLowerCase()

通过 Set.has(ext) 判断
是否属于当前媒体类型。

匹配：

参与随机抽样。

不匹配：

跳过。

----------------------------------------
M09.08 水库抽样
----------------------------------------

使用 Reservoir Sampling。

内部状态：

selected: string[]
seen: number

每遇到一个符合条件的媒体：

seen++

当 selected 长度小于 10：

直接加入当前媒体路径。

当 selected 已满：

调用：

crypto.randomInt(seen)

得到：

j

如果：

j < pageSize

则替换：

selected[j] = filePath

因此抽样不依赖预先得到
整个媒体目录的完整文件列表。

----------------------------------------
M09.09 相对路径生成
----------------------------------------

完整文件路径：

fullPath

媒体根路径：

rootPath

代码：

fullPath.replace(rootPath, "")

得到用于构造访问 URL 的路径字符串。

----------------------------------------
M09.10 媒体 URL 生成
----------------------------------------

读取配置：

server.staticURL

每个抽样结果生成：

staticURL
+
"/"
+
encodeURIComponent(e.slice(1))

encodeURIComponent 对媒体相对路径
作为整体字符串进行编码。

----------------------------------------
M09.11 列表响应
----------------------------------------

返回：

{
  "data": [
    "<媒体URL1>",
    "<媒体URL2>",
    "<媒体URL3>"
  ]
}

数组元素是媒体访问 URL 字符串。

不是媒体元数据对象。

----------------------------------------
M09.12 目录错误处理
----------------------------------------

traverse() 使用：

try / catch / finally

读取失败时：

console.error()

finally 中：

dirHandle.close()

外层 list() 也有额外 try/catch。

最后返回当前 selected 集合。

----------------------------------------
M09.13 当前列表语义
----------------------------------------

现有实现的行为：

- 每次请求重新遍历目录。
- 每次请求独立执行抽样。
- 没有分页页码。
- 没有游标。
- 没有已展示媒体历史状态。
- 没有媒体 ID 数据库。
- 没有媒体排序参数。
- 没有目录筛选参数。
- 没有标签或收藏条件。
- 没有区分普通视频列表和推荐视频列表的后端接口。

每次调用的结果都是一组新的随机样本。


========================================
M10 - MEDIA STREAMING
媒体文件流与 Range 请求
========================================

源文件：

server/src/modules/stream/stream.controller.ts
server/src/modules/stream/stream.service.ts

----------------------------------------
M10.01 文件流 API
----------------------------------------

HTTP：

GET /api/stream/*

Controller：

StreamController.streamPublicFile()

使用通配符路径参数。

输入：

@Param("*") file

@Res() res

@Headers("range") range

服务调用：

StreamService.streamFile(
  file,
  res,
  range
)

接口名称虽然包含 Public，
但 Controller 没有 @Public 装饰器。

----------------------------------------
M10.02 文件路径解码
----------------------------------------

入口参数：

filePath

处理：

decodeURIComponent(filePath)

然后：

join(this.workDir, decodedPath)

生成文件系统路径。

----------------------------------------
M10.03 文件存在性检查
----------------------------------------

调用：

existsSync(path)

文件不存在时：

NotFoundException("File not found")

----------------------------------------
M10.04 文件元数据
----------------------------------------

通过 statSync 读取：

fileSize

通过 extname 取得：

ext

通过 basename 取得：

filename

使用：

getContentType(ext)

得到文件 Content-Type。

----------------------------------------
M10.05 文件响应头
----------------------------------------

设置：

Access-Control-Allow-Origin: *

Access-Control-Allow-Methods:
GET, HEAD

Access-Control-Expose-Headers:
Content-Length,Content-Range

Access-Control-Allow-Headers:
Range

Accept-Ranges:
bytes

----------------------------------------
M10.06 Content-Disposition
----------------------------------------

对文件名进行：

encodeURIComponent(filename)

设置：

Content-Disposition:
inline; filename="<encoded filename>"

浏览器收到的是 inline 类型内容。

----------------------------------------
M10.07 HEAD 请求分支
----------------------------------------

streamFile() 内部检查：

res.request.method === "HEAD"

执行：

Content-Type
Content-Length

然后结束响应。

当前源码显式声明的是 GET 路由；
HEAD 处理逻辑位于共享文件流方法内部。

----------------------------------------
M10.08 普通文件传输
----------------------------------------

当没有 Range Header：

设置：

Content-Type
Content-Length

调用：

createReadStream(path)

并执行：

stream.pipe(res.raw)

返回完整文件内容。

----------------------------------------
M10.09 Range 请求解析
----------------------------------------

当存在 Range Header：

调用：

range.replace(/bytes=/, "").split("-")

解析：

start
end

start：

parseInt(parts[0], 10)

end：

如果存在第二部分，
使用 parseInt()。

否则：

fileSize - 1

----------------------------------------
M10.10 Range 边界判断
----------------------------------------

当前检查：

start >= fileSize

或者：

end >= fileSize

符合条件时：

设置：

Content-Range: bytes */fileSize

返回：

HTTP 416

----------------------------------------
M10.11 部分内容长度
----------------------------------------

对 end 进行：

Math.min(end, fileSize - 1)

计算：

contentLength = end - start + 1

----------------------------------------
M10.12 部分内容响应
----------------------------------------

返回：

HTTP 206 Partial Content

响应头：

Content-Type

Content-Length

Content-Range:
bytes start-end/fileSize

Cache-Control:
public, max-age=31536000

----------------------------------------
M10.13 范围文件读取
----------------------------------------

使用：

createReadStream(path, {
  start,
  end
})

然后：

stream.pipe(res.raw)

只读取指定字节区间。

----------------------------------------
M10.14 文件流错误事件
----------------------------------------

Range 流注册：

stream.on("error")

出现流错误时：

console.error()

如果响应头尚未发送：

返回：

HTTP 500
Internal Server Error

----------------------------------------
M10.15 同步异常捕获
----------------------------------------

整个 streamFile() 主逻辑位于：

try / catch

发生 NotFoundException：

返回 404。

发生其他同步异常：

返回 500。

----------------------------------------
M10.16 当前媒体传输能力
----------------------------------------

已实现的传输逻辑：

- 文件完整读取
- 文件流式输出
- 字节区间读取
- 206 响应
- 416 响应
- HEAD 分支
- MIME 类型响应
- 文件名响应头
- 跨域响应头
- 部分内容缓存头

没有独立的：

- 视频转码管线
- HLS 分片生成
- DASH 分片生成
- 媒体封面生成
- 音视频格式转换
- 媒体压缩任务


========================================
M11 - STATIC WEB HOSTING
前端静态文件托管
========================================

源文件：

server/src/app.module.ts
server/entrypoint.sh
Dockerfile

----------------------------------------
M11.01 静态托管模块
----------------------------------------

使用：

ServeStaticModule.forRoot()

依赖：

@nestjs/serve-static
@fastify/static

负责由后端服务前端构建产物。

----------------------------------------
M11.02 开发环境路径
----------------------------------------

当：

NODE_ENV === "development"

rootPath：

join(
  __dirname,
  "../..",
  "client",
  "dist"
)

----------------------------------------
M11.03 其他环境路径
----------------------------------------

rootPath：

/workspace/client/dist

----------------------------------------
M11.04 前端运行时配置生成
----------------------------------------

入口脚本：

server/entrypoint.sh

容器启动时写入：

/workspace/client/dist/env.js

内容结构：

window.__APP_CONFIG__ = {
  VITE_API_URL: "...",
  VITE_API_PORT: "..."
};

对应变量：

VITE_API_URL
VITE_API_PORT

----------------------------------------
M11.05 前后端交付关系
----------------------------------------

Docker 构建中：

client-builder

生成前端 dist。

随后复制到：

/workspace/client/dist

NestJS 在运行时托管这个目录。

因此现有生产容器同时包含：

- 后端 HTTP 服务
- 前端构建后的静态文件


========================================
M12 - ERROR HANDLING & LOGGING
异常处理与日志
========================================

源文件：

server/src/filters/error-exception.filter.ts
server/src/filters/http-exception.filter.ts
server/src/filters/index.ts
server/src/main.ts

----------------------------------------
M12.01 HTTP 异常过滤器
----------------------------------------

类：

HttpExceptionFilter

装饰器：

@Catch(HttpException)

处理 NestJS HttpException。

----------------------------------------
M12.02 HTTP 异常信息
----------------------------------------

提取：

exception.getStatus()

提取错误消息：

exception.getResponse().message

如果不存在：

exception.message

----------------------------------------
M12.03 HTTP 异常日志
----------------------------------------

记录：

HTTP Method
Request URL
错误消息

使用：

Logger.error()

----------------------------------------
M12.04 HTTP 异常响应
----------------------------------------

格式：

{
  "code": "<HTTP状态码>",
  "data": null,
  "message": "<错误消息>"
}

响应 HTTP 状态码与异常状态码一致。

----------------------------------------
M12.05 通用 Error 过滤器
----------------------------------------

类：

ErrorExceptionsFilter

装饰器：

@Catch(Error)

负责处理 Error 类型异常。

----------------------------------------
M12.06 通用 Error 日志
----------------------------------------

记录：

HTTP Method
Request URL
exception.message

----------------------------------------
M12.07 通用 Error 响应
----------------------------------------

HTTP：

500 Internal Server Error

格式：

{
  "code": 500,
  "data": null,
  "message": "<exception.message>"
}

----------------------------------------
M12.08 过滤器统一导出
----------------------------------------

文件：

filters/index.ts

导出：

HttpExceptionFilter
ErrorExceptionsFilter

----------------------------------------
M12.09 业务日志来源
----------------------------------------

AuthService：

登录成功日志。

IPGuard：

拒绝访问日志。

JWTGuard：

Header 不匹配时使用 console.log。

StreamService：

目录读取错误、
文件读取错误、
流错误日志。

main.ts：

启动日志、
未捕获异常、
未处理 Promise 拒绝。

----------------------------------------
M12.10 返回值风格
----------------------------------------

正常业务响应：

直接返回 Controller / Service 的对象。

例如：

验证码：
{id, data}

登录：
{token}

媒体列表：
{data}

异常响应：

由 ExceptionFilter 返回：
{code, data, message}

两者是当前后端不同的响应结构。


========================================
M13 - COMMON TYPES & UTILITIES
公共工具和类型
========================================

源文件：

server/src/utils/tool.utils.ts
server/src/types/interfaces/auth.interface.ts
server/src/decorators/

----------------------------------------
M13.01 UUID 生成
----------------------------------------

函数：

generateUUID()

无参数。

返回：

string

通过固定格式：

xxxxxxxx-xxxx-xxxx-yxxx-xxxxxxxxxxxx

生成 UUID 样式字符串。

随机部分来自：

Math.random()

使用位置：

CodeService.getUniqueUUID()

TokenService.generateToken()

----------------------------------------
M13.02 IPv4 数值转换
----------------------------------------

函数：

ip4ToNum(ip)

私有辅助函数。

输入：

IPv4 字符串。

输出：

32 位无符号整数。

使用位置：

isAllowedIPs()

----------------------------------------
M13.03 IP 地址匹配
----------------------------------------

函数：

isAllowedIPs(
  ip,
  AllowedIPs,
  allowIPv6
)

输入：

目标 IP
白名单字符串
IPv6 开关

输出：

boolean

使用位置：

IPGuard

----------------------------------------
M13.04 JWT 类型
----------------------------------------

接口名称：

JWT

字段：

host
connection
user-agent
dnt
accept
sec-fetch-site
referer
accept-language
session
ip
iat?
exp?

该类型用于 JWTGuard 的请求校验逻辑。

----------------------------------------
M13.05 Public 装饰器
----------------------------------------

定义：

IS_PUBLIC_KEY

值：

"isPublic"

函数：

Public()

功能：

在路由或 Controller 上设置公开访问元数据。

消费者：

JWTGuard

----------------------------------------
M13.06 JWT 参数装饰器
----------------------------------------

函数：

JWT()

功能：

读取经过校验的 request.jwt，
或者其中指定的属性。

当前仅有定义，
没有发现业务 Controller 使用。


========================================
M14 - BUILD & DEPLOYMENT
构建、容器与进程运行
========================================

源文件：

Dockerfile
server/entrypoint.sh
server/ecosystem.config.js
build.sh
server/package.json
server/nest-cli.json
server/tsconfig.json
server/tsconfig.build.json

----------------------------------------
M14.01 后端开发与构建命令
----------------------------------------

package.json 定义：

build：
nest build

start：
nest start

start:dev：
nest start --watch

start:debug：
nest start --debug --watch

start:prod：
node dist/main

dev：
NODE_ENV=development nest start --watch

format：
prettier

lint：
eslint --fix

test：
jest

test:watch：
jest --watch

test:cov：
jest --coverage

test:e2e：
jest --config ./test/jest-e2e.json

----------------------------------------
M14.02 TypeScript 编译
----------------------------------------

server/tsconfig.json

主要配置：

module: commonjs
target: ES2023
outDir: ./dist
sourceMap: true
declaration: true

启用：

emitDecoratorMetadata
experimentalDecorators

路径别名：

@/* -> src/*

----------------------------------------
M14.03 NestJS CLI
----------------------------------------

server/nest-cli.json

sourceRoot：

src

构建时：

deleteOutDir: true

----------------------------------------
M14.04 Docker 多阶段构建
----------------------------------------

Dockerfile 分为：

Stage 1：
client-builder

Stage 2：
server-builder

Stage 3：
Runtime

使用：

node:22-alpine

----------------------------------------
M14.05 前端构建阶段
----------------------------------------

执行：

npm ci

npm run build

生成：

client/dist

----------------------------------------
M14.06 后端构建阶段
----------------------------------------

执行：

npm ci

npm run build

npm prune --omit=dev

保留后端运行需要的生产依赖。

----------------------------------------
M14.07 最终镜像
----------------------------------------

工作目录：

/workspace

复制：

前端 dist
后端 dist
后端生产依赖
环境配置匹配文件
PM2 配置
入口脚本

入口：

/workspace/entrypoint.sh

----------------------------------------
M14.08 Docker 默认环境变量
----------------------------------------

Dockerfile 声明：

NODE_ENV
SSL
ALLOWED_IPS
ALLOW_IPv6
VITE_API_URL
VITE_API_PORT
ACCOUNT
PASSWORD
JWT_SECRET
JWT_EXPIRE

其中：

NODE_ENV=production

SSL=true

VITE_API_PORT=3000

JWT_EXPIRE=1200

----------------------------------------
M14.09 容器启动脚本
----------------------------------------

entrypoint.sh 先生成前端：

env.js

再执行：

pm2-runtime start ecosystem.config.js

----------------------------------------
M14.10 PM2 运行模式
----------------------------------------

应用名称：

nas-tiktok

工作目录：

/workspace/server

启动文件：

./dist/main.js

模式：

cluster

实例数：

max

自动重启：

true

文件监听：

false

----------------------------------------
M14.11 PM2 内存配置
----------------------------------------

max_memory_restart：

400M

Node 参数：

--max-old-space-size=384

----------------------------------------
M14.12 PM2 重启配置
----------------------------------------

min_uptime：

60s

max_restarts：

10

restart_delay：

5000ms

listen_timeout：

10000ms

kill_timeout：

3000ms

----------------------------------------
M14.13 本地构建脚本
----------------------------------------

根目录：

build.sh

主要函数：

get_latest_semantic_version_tag()

increment_patch_version()

build()

deploy()

export_docker_image()

import_docker_image()

----------------------------------------
M14.14 镜像版本
----------------------------------------

镜像名称：

nas-tiktok

标签格式：

vMAJOR.MINOR.PATCH

通过本地已有镜像标签查找最高语义版本。

每次 build()：

增加 PATCH 版本。

----------------------------------------
M14.15 构建流程
----------------------------------------

当前 build()：

1. 检查源码目录。
2. 不存在则 git clone。
3. 获取当前镜像最高版本。
4. 增加 PATCH。
5. 询问是否使用构建代理。
6. 执行 docker build。
7. 可选删除旧镜像。
8. 可选导出镜像。

----------------------------------------
M14.16 部署流程
----------------------------------------

当前 deploy()：

1. 选择镜像 TAG。
2. 检查旧容器。
3. 停止旧容器。
4. 删除旧容器。
5. 创建工作目录。
6. 创建新容器。
7. 挂载证书目录。
8. 挂载媒体目录。
9. 设置重启策略。
10. 询问是否启动。
11. 执行 docker start。

挂载：

WORK_DIR/certs
    -> /workspace/certs

WORK_DIR/statics
    -> /workspace/statics

----------------------------------------
M14.17 Docker 镜像导出
----------------------------------------

函数：

export_docker_image()

使用：

docker save

文件格式：

nas-tiktok_vX.Y.Z.tar

----------------------------------------
M14.18 Docker 镜像导入
----------------------------------------

函数：

import_docker_image()

从源码目录寻找修改时间最新的 tar 文件。

调用：

docker load

之后从本地镜像中查找语义版本标签。

----------------------------------------
M14.19 构建脚本交互选项
----------------------------------------

菜单：

a. 构建
b. 部署
c. 构建 + 部署
d. 导出镜像
e. 导入镜像

通过 Bash case 分发。


========================================
三、完整 HTTP API 清单
========================================

当前业务 Controller 共定义 4 个路由。

----------------------------------------
API-01 获取验证码
----------------------------------------

Method：

GET

Path：

/api/captcha

认证：

JWTGuard 公开例外

IP：

仍属于 IPGuard 检查范围

输入：

无业务参数。

输出：

{
  "id": "string",
  "data": "data:image/svg+xml;base64,..."
}

核心调用：

AuthController.captcha()
  -> CodeService.captcha()
  -> getUniqueUUID()
  -> CacheManager.set()

----------------------------------------
API-02 用户登录
----------------------------------------

Method：

POST

Path：

/api/logIn

认证：

JWTGuard 公开例外

其他校验：

CodeGuard

输入：

{
  "account": "string",
  "password": "string",
  "codeId": "string",
  "code": "string"
}

输出：

{
  "token": "string"
}

核心调用：

AuthController.logIn()
  -> CodeGuard.canActivate()
  -> CodeService.checkVerifyCode()
  -> AuthService.logIn()
  -> ConfigService.get("account")
  -> TokenService.generateToken()
  -> JwtService.signAsync()

----------------------------------------
API-03 随机媒体列表
----------------------------------------

Method：

GET

Path：

/api/list

Query：

type=image
type=video

认证：

JWT

通常使用：

Authorization: Bearer <token>

输出：

{
  "data": [
    "https://.../api/stream/...",
    "https://.../api/stream/..."
  ]
}

最多：

10 个媒体 URL。

核心调用：

StreamController.list()
  -> StreamService.list()
  -> traverse()
  -> opendir()
  -> randomInt()
  -> URL 编码
  -> 返回列表

----------------------------------------
API-04 媒体文件流
----------------------------------------

Method：

GET

Path：

/api/stream/*

认证：

JWT

当前媒体模式：

?auth=<token>

输入：

路径通配参数

可选 Header：

Range

输出：

- 文件内容
- 文件字节区间
- 相应 Content-Type
- 相应 Content-Length
- 相应 Content-Range

主要响应状态：

200
206
404
416
500

核心调用：

StreamController.streamPublicFile()
  -> StreamService.streamFile()
  -> decodeURIComponent()
  -> path.join()
  -> existsSync()
  -> statSync()
  -> getContentType()
  -> createReadStream()
  -> HTTP Response

服务方法还包含 HEAD 请求分支，
但没有单独定义 @Head() Controller 路由。


========================================
四、完整数据流
========================================

----------------------------------------
FLOW-01 应用启动
----------------------------------------

Docker / npm
    |
    v
main.ts
    |
    v
NestFactory
    |
    v
FastifyAdapter
    |
    v
AppModule
    |
    +--> ConfigModule
    |
    +--> CacheModule
    |
    +--> ServeStaticModule
    |
    +--> AuthModule
    |
    +--> StreamModule
    |
    +--> JWTGuard
    |
    +--> IPGuard
    |
    v
app.listen()


----------------------------------------
FLOW-02 验证码生成
----------------------------------------

GET /api/captcha
    |
    v
IPGuard
    |
    v
JWTGuard 检测 @Public
    |
    v
AuthController
    |
    v
CodeService
    |
    +--> svg-captcha
    |
    +--> UUID
    |
    +--> CacheManager
    |
    v
{id, data}


----------------------------------------
FLOW-03 登录认证
----------------------------------------

POST /api/logIn
    |
    v
全局 Guard 处理
    |
    v
CodeGuard
    |
    v
CodeService
    |
    v
验证码缓存校验
    |
    v
AuthService
    |
    v
环境变量账号密码校验
    |
    v
TokenService
    |
    v
JwtService.signAsync()
    |
    v
{token}


----------------------------------------
FLOW-04 媒体列表
----------------------------------------

GET /api/list
    |
    v
IPGuard / JWTGuard
    |
    v
StreamController
    |
    v
StreamService.list()
    |
    v
定位媒体目录
    |
    v
递归读取目录
    |
    v
检查扩展名
    |
    v
水库抽样
    |
    v
构造媒体 URL
    |
    v
{data: [...]}


----------------------------------------
FLOW-05 媒体播放
----------------------------------------

GET /api/stream/*
    |
    v
IPGuard / JWTGuard
    |
    v
StreamController
    |
    v
StreamService.streamFile()
    |
    v
解析文件路径
    |
    v
读取文件属性
    |
    v
选择 Content-Type
    |
    +--> 无 Range
    |       |
    |       v
    |    完整文件流
    |
    +--> 有 Range
    |       |
    |       v
    |    解析字节范围
    |       |
    |       v
    |    206 部分内容
    |
    +--> HEAD 分支
            |
            v
         响应文件头


----------------------------------------
FLOW-06 前端静态托管
----------------------------------------

Docker client-builder
    |
    v
Vue 构建产物
    |
    v
/workspace/client/dist
    |
    v
entrypoint.sh 生成 env.js
    |
    v
ServeStaticModule
    |
    v
HTTP 静态资源响应


========================================
五、源文件与功能归属映射
========================================

server/src/main.ts

归属：
M01、M03、M12

职责：
应用启动、HTTPS、全局管道、
CORS、异常处理注册、HTTP 监听。

----------------------------------------

server/src/app.module.ts

归属：
M01、M02、M03、M11

职责：
模块组合、全局缓存、
全局配置、静态服务、全局 Guard。

----------------------------------------

server/src/configs/server.config.ts

归属：
M02

职责：
服务地址、监听端口、
IPv6、IP 配置、媒体基础 URL。

----------------------------------------

server/src/configs/account.config.ts

归属：
M02、M06

职责：
账户和密码配置。

----------------------------------------

server/src/configs/jwt.config.ts

归属：
M02、M07

职责：
JWT 密钥和过期时间配置。

----------------------------------------

server/src/modules/auth/auth.module.ts

归属：
M05、M06、M07

职责：
Auth Controller、Service、
验证码服务、令牌服务、JwtModule 注册。

----------------------------------------

server/src/modules/auth/auth.controller.ts

归属：
M05、M06

职责：
验证码和登录 HTTP 接口。

----------------------------------------

server/src/modules/auth/auth.dto.ts

归属：
M05、M06

职责：
验证码和登录请求数据类型与字段验证。

----------------------------------------

server/src/modules/auth/auth.service.ts

归属：
M06

职责：
环境账号密码检查、
登录日志、令牌生成调用。

----------------------------------------

server/src/modules/auth/code.service.ts

归属：
M05

职责：
生成数学验证码、
生成验证码 ID、
缓存验证码、
校验和删除验证码。

----------------------------------------

server/src/modules/auth/token.service.ts

归属：
M07

职责：
生成 Session UUID、
组装 JWT Payload、
签发 JWT。

----------------------------------------

server/src/guards/code.guard.ts

归属：
M05

职责：
登录接口验证码请求检查。

----------------------------------------

server/src/guards/ip.guard.ts

归属：
M04

职责：
读取客户端 IP、
执行白名单匹配、
拒绝未通过请求。

----------------------------------------

server/src/guards/jwt.guard.ts

归属：
M07

职责：
公开路由检测、
JWT 提取、
签名验证、
HTTP Header 与 IP 比较、
保存 request.jwt。

----------------------------------------

server/src/decorators/public.decorator.ts

归属：
M07、M13

职责：
标记 JWT 公开路由。

----------------------------------------

server/src/decorators/jwt.decorator.ts

归属：
M07、M13

职责：
提取请求 JWT Payload。

----------------------------------------

server/src/modules/stream/stream.modules.ts

归属：
M08、M09、M10

职责：
注册媒体 Controller 和 Service。

----------------------------------------

server/src/modules/stream/stream.controller.ts

归属：
M09、M10

职责：
媒体列表接口、
媒体流接口。

----------------------------------------

server/src/modules/stream/stream.service.ts

归属：
M08、M09、M10

职责：
媒体目录定位、
文件扩展名集合、
MIME 类型、
递归扫描、
随机抽样、
媒体 URL 构造、
文件流、
Range 解析、
HTTP 文件响应。

----------------------------------------

server/src/filters/error-exception.filter.ts

归属：
M12

职责：
通用 Error 处理。

----------------------------------------

server/src/filters/http-exception.filter.ts

归属：
M12

职责：
HTTP 异常处理。

----------------------------------------

server/src/filters/index.ts

归属：
M12

职责：
异常过滤器统一导出。

----------------------------------------

server/src/types/interfaces/auth.interface.ts

归属：
M07、M13

职责：
JWT Payload 类型声明。

----------------------------------------

server/src/utils/tool.utils.ts

归属：
M04、M05、M07、M13

职责：
UUID 生成、
IPv4 数值转换、
IP 地址范围判断。

----------------------------------------

server/entrypoint.sh

归属：
M11、M14

职责：
生成前端运行时配置、
启动 PM2。

----------------------------------------

server/ecosystem.config.js

归属：
M14

职责：
PM2 进程数量、
运行模式、
重启条件、
内存限制。

----------------------------------------

Dockerfile

归属：
M11、M14

职责：
构建前端、
构建后端、
生成运行镜像、
准备运行环境。

----------------------------------------

build.sh

归属：
M14

职责：
镜像构建、
版本递增、
容器部署、
镜像导入和导出。


========================================
六、现有测试与开发辅助文件
========================================

TEST-01：

server/test/app.e2e-spec.ts

使用：

@nestjs/testing
supertest

现有测试描述：

AppController (e2e)

请求：

GET /

期望：

HTTP 200
Hello World!

这是当前仓库中的测试内容。

----------------------------------------

TEST-02：

server/test/jest-e2e.json

提供：

Jest E2E 测试配置。

----------------------------------------

TEST-03：

server/package.json

包含：

Jest 单元测试配置。

----------------------------------------

DEV-01：

server/eslint.config.mjs

提供：

ESLint
TypeScript ESLint
Prettier

相关规则配置。

----------------------------------------

DEV-02：

server/.prettierrc

定义：

singleQuote: true
trailingComma: all


========================================
七、后端现有数据对象清单
========================================

OBJECT-01 AccountConfig

{
  account: string,
  password: string
}

----------------------------------------

OBJECT-02 JWTConfig

{
  accessSecret: string,
  accessExpiresIn: number
}

----------------------------------------

OBJECT-03 ServerConfig

{
  appName: string,
  appURL: string,
  urlPrefix: string,
  host: string,
  port: number,
  staticURL: string,
  allowIPv6: boolean,
  allowedIPs: string
}

----------------------------------------

OBJECT-04 VerifyCodeDto

{
  codeId?: string,
  code?: string
}

----------------------------------------

OBJECT-05 AuthDto

{
  account: string,
  password: string,
  codeId?: string,
  code?: string
}

----------------------------------------

OBJECT-06 CaptchaResponse

{
  id: string,
  data: string
}

----------------------------------------

OBJECT-07 LoginResponse

{
  token: string
}

----------------------------------------

OBJECT-08 JWT Payload

{
  host,
  connection,
  "user-agent",
  dnt,
  "sec-fetch-site",
  "accept-language",
  session,
  ip,
  iat,
  exp
}

其中部分 HTTP Header 字段
是否存在取决于请求上下文。

----------------------------------------

OBJECT-09 MediaListRequest

{
  type: "image" | "video"
}

----------------------------------------

OBJECT-10 MediaListResponse

{
  data: string[]
}

----------------------------------------

OBJECT-11 MediaStreamInput

{
  filePath: string,
  range?: string
}

----------------------------------------

OBJECT-12 ErrorResponse

{
  code: number,
  data: null,
  message: string | string[]
}

----------------------------------------

OBJECT-13 RuntimeWebConfig

window.__APP_CONFIG__ = {
  VITE_API_URL,
  VITE_API_PORT
}


========================================
八、跨模块依赖关系
========================================

Application Runtime
    |
    +--> Configuration
    |
    +--> HTTP Infrastructure
    |
    +--> Network Access Control
    |
    +--> JWT & Session
    |
    +--> Static Web Hosting
    |
    +--> Error Handling


Account Authentication
    |
    +--> AccountConfig
    |
    +--> Captcha Guard
    |
    +--> TokenService
              |
              +--> JwtService
              |
              +--> UUID Utility


Captcha
    |
    +--> svg-captcha
    |
    +--> CacheManager
    |
    +--> UUID Utility


JWT Verification
    |
    +--> JWTConfig
    |
    +--> JwtService
    |
    +--> Public Metadata
    |
    +--> HTTP Request Context


Network Access Control
    |
    +--> ServerConfig
    |
    +--> isAllowedIPs()
              |
              +--> ip4ToNum()
              |
              +--> net.isIPv4()
              |
              +--> net.isIPv6()


Media Discovery
    |
    +--> Media Storage
    |
    +--> ServerConfig.staticURL
    |
    +--> File System
    |
    +--> crypto.randomInt()


Media Streaming
    |
    +--> Media Storage
    |
    +--> File System
    |
    +--> MIME Type Lookup
    |
    +--> HTTP Response


Static Web Hosting
    |
    +--> Frontend Dist
    |
    +--> Runtime env.js


Build & Deployment
    |
    +--> Docker
    |
    +--> Node.js
    |
    +--> PM2
    |
    +--> TLS Certificate Directory
    |
    +--> NAS Media Directory


========================================
九、完整功能边界
========================================

以下是当前后端源码中能够确认的功能。

APPLICATION：

[+] HTTP 服务
[+] HTTPS 服务配置
[+] HTTP/2 配置
[+] IPv4 监听
[+] IPv6 监听
[+] 环境变量加载
[+] 全局 API 路由前缀
[+] CORS 配置
[+] 全局数据验证
[+] 全局异常处理
[+] 进程异常监听

AUTHENTICATION：

[+] 单账户登录
[+] 环境变量账号密码
[+] 数学图形验证码
[+] SVG 验证码图像
[+] 验证码缓存
[+] 验证码有效期
[+] 验证码一次性删除逻辑
[+] JWT 生成
[+] JWT 有效期
[+] JWT 签名验证
[+] Session UUID
[+] 请求 Header 比对
[+] 客户端 IP 比对
[+] Public 路由标记
[+] JWT 参数装饰器

NETWORK：

[+] IP 白名单开关逻辑
[+] IPv4 地址检查
[+] IPv4 CIDR 范围匹配
[+] IPv6 地址识别
[+] Loopback 特殊处理
[+] 禁止访问异常及日志

MEDIA DISCOVERY：

[+] NAS 本地文件系统扫描
[+] 多级目录递归
[+] @eaDir 跳过
[+] 图片扩展名识别
[+] 视频扩展名识别
[+] 文件类型筛选
[+] 水库抽样
[+] 单次最多 10 个结果
[+] 媒体 URL 生成
[+] URL 路径编码
[+] 目录读取异常记录

MEDIA STREAMING：

[+] 媒体文件存在性检查
[+] 文件大小读取
[+] MIME 类型识别
[+] 文件完整读取
[+] 文件流式传输
[+] Range 字节区间读取
[+] HTTP 206
[+] HTTP 416
[+] HEAD 请求处理分支
[+] Content-Length
[+] Content-Range
[+] Accept-Ranges
[+] Content-Disposition
[+] 媒体响应 CORS Header
[+] 部分响应缓存头

WEB HOSTING：

[+] 前端 dist 静态托管
[+] 开发/生产路径区分
[+] 运行时 env.js 生成

DEPLOYMENT：

[+] Docker 多阶段构建
[+] Node.js 生产环境
[+] PM2 Cluster
[+] 自动重启
[+] 内存重启阈值
[+] Docker 镜像版本递增
[+] Docker 镜像导出
[+] Docker 镜像导入
[+] Docker 容器创建
[+] NAS 媒体目录挂载
[+] TLS 证书目录挂载


========================================
十、当前源码未定义的业务功能
========================================

以下内容不属于目前已实现的后端业务接口。

用户相关：

- 用户注册
- 用户资料管理
- 多账户数据库
- RBAC 角色权限
- 密码重置
- Refresh Token 接口
- Logout 接口
- 会话列表

媒体管理：

- 媒体上传
- 媒体删除
- 媒体重命名
- 媒体移动
- 元数据数据库
- 媒体搜索
- 收藏记录
- 点赞记录
- 播放历史记录
- 观看进度持久化
- 缩略图生成
- 视频转码
- HLS/DASH 转换

推荐相关：

- 用户画像
- 推荐权重数据库
- 推荐历史记录
- 推荐反馈接口
- 内容评分
- 独立推荐算法服务

任务相关：

- 后台任务队列
- 定时索引任务
- 数据库迁移
- 独立 Worker API
- WebSocket 业务接口

当前媒体推荐行为由随机抽样实现，
不是基于用户行为的推荐系统。


========================================
十一、最终功能模块索引
========================================

模块编号 | 功能领域 | 当前源码归属

M01
Application Runtime
main.ts / app.module.ts

M02
Configuration
configs/ / app.module.ts

M03
HTTP Infrastructure
main.ts / app.module.ts

M04
Network Access Control
ip.guard.ts / tool.utils.ts

M05
Captcha
code.service.ts / code.guard.ts

M06
Account Authentication
auth.controller.ts / auth.service.ts / auth.dto.ts

M07
JWT & Session
token.service.ts / jwt.guard.ts / jwt.config.ts

M08
Media Storage
stream.service.ts 的路径、类型和文件属性逻辑

M09
Media Discovery & Sampling
stream.service.ts 的 list() 与 traverse()

M10
Media Streaming
stream.service.ts 的 streamFile()

M11
Static Web Hosting
ServeStaticModule / entrypoint.sh

M12
Error Handling & Logging
filters/ 及各业务 Logger

M13
Common Types & Utilities
utils/ / types/ / decorators/

M14
Build & Deployment
Dockerfile / ecosystem.config.js /
entrypoint.sh / build.sh


========================================
十二、源码整体事实总结
========================================

NAS-TikTok 的后端是一个以文件系统为
媒体来源的 HTTP 服务。

核心业务围绕：

账户认证
+
访问控制
+
文件发现
+
随机抽样
+
文件流式读取

展开。

当前正式 NestJS 业务模块数量为 2：

AuthModule
StreamModule

当前业务 API 数量为 4：

GET  /api/captcha
POST /api/logIn
GET  /api/list
GET  /api/stream/*

此外，还包含：

- 全局配置服务
- 全局 IP Guard
- 全局 JWT Guard
- 全局异常过滤器
- 公共工具函数
- 静态前端托管
- 容器与进程运行配置

当前代码没有数据库持久化业务系统，
也没有独立的媒体索引数据库。

媒体列表每次通过文件系统扫描，
使用水库抽样生成随机媒体 URL。

媒体内容由原始文件读取并通过 HTTP 流式返回。

客户端能够通过普通文件请求或 Range 请求
读取图片及视频。

账户凭据来自环境配置。

验证码答案存储在缓存中。

JWT 使用配置密钥签发，
并与请求上下文中的部分 HTTP Header
以及客户端 IP 建立关联。

生产运行结构包含：

Docker
+
Node.js
+
PM2 Cluster
+
NestJS/Fastify
+
Vue 静态构建产物

以上为当前 dev 分支固定 Commit
所包含的后端功能事实与逻辑归属。

========================================
END OF BACKEND FUNCTIONAL INVENTORY
========================================
