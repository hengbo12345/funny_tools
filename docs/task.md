# GPS 信号模拟测试工具开发任务清单

- [x] 初始化 Go 项目与依赖 (`go.mod`)
- [x] 准备内置默认星历文件 `data/ephemeris/brdc1350.26n`
- [x] 编写 Go 后端逻辑 `main.go`
  - [x] 基础 HTTP 服务器与静态资源嵌入 (`go:embed`)
  - [x] 进程管理器：安全运行外部命令、获取实时输出、终止进程
  - [x] API 路由与控制器 (状态获取、启动/停止模拟、日志 SSE 推送、星历下载/上传)
  - [x] 自动星历管理器 (NOAA CORS 每日星历自动下载与解压)
  - [x] 系统环境依赖检查与一键安装编译逻辑 (`gps-sdr-sim`)
- [x] 编写前端静态页面 `static/index.html`
- [x] 编写前端样式 `static/app.css` (高颜值暗黑玻璃微影风格)
- [x] 编写前端逻辑 `static/app.js` (地图点击/拖拽选点、Nominatim 地址解析、日志流接收)
- [x] 本地跨平台编译测试与打包验证
- [x] 撰写部署与使用说明 `README.md` 与生成 `walkthrough.md`
