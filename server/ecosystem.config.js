export const apps = [
  {
    name: 'nas-tiktok', // 应用名称
    cwd: '/workspace/server', // 当前工作目录，通常是构建后代码所在位置
    script: './dist/main.js', // 启动脚本（编译后的入口）
    /* 进程管理 */
    exec_mode: 'cluster', // 使用 cluster 模式（推荐用于无状态服务）
    instances: 'max', // 根据 CPU 核数自动启动多个进程
    autorestart: true, // 程序崩溃后自动重启
    watch: false, // 关闭监听文件变化（生产环境应关闭）
    max_memory_restart: '400M', // PM2 单个进程内存超出 400MB 自动重启，防止内存泄漏崩溃
    /* Node 启动参数 */
    exec_interpreter: 'node', // 使用 node 作为解释器
    node_args: '--max-old-space-size=384', // 设置 Node 最大内存使用为 384MB，保留部分系统开销
    /* 环境变量 */
    env: {
      NODE_ENV: 'production', // 设置环境变量为 production
    },
    /* 重启策略 */
    min_uptime: '60s', // 进程至少运行 60 秒才算成功启动
    max_restarts: 10, // 最大重启次数（超过则停止尝试）
    restart_delay: 5000, // 每次重启之间的延迟（毫秒）
    /* 启动/关闭超时控制 */
    listen_timeout: 10000, // 等待应用开始监听端口的时间（毫秒）
    kill_timeout: 3000, // 关闭进程前的最大等待时间（毫秒）
    /* 日志 */
    // output: '/dev/stdout', // 标准输出（正常日志）
    // error: '/dev/stderr', // 错误输出（异常、崩溃）
    // merge_logs: true, // 合并多实例日志到同一个流（stdout/stderr）
    // log_date_format: 'YYYY-MM-DD HH:mm:ss', // 可选：日志时间格式更可读
  },
];
