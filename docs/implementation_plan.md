# GPS 信号模拟测试工具设计与实现计划

本项目旨在基于 **HackRF One** 设备和 **Ubuntu Server** 环境，开发一个高颜值、极简部署的 **GPS 信号模拟测试工具**。用户可以通过 Web 界面搜索地址或在地图上选点，一键触发 HackRF One 发射 GPS 模拟信号。

## 技术栈选择

1. **后端服务 (Go 语言)**:
   - 使用 Go 开发单文件 Web 服务，利用 `go:embed` 将前端静态资源（HTML/CSS/JS）打包进二进制文件中。
   - 部署极其轻量，支持在 Mac 上交叉编译，在 Ubuntu Server 上无依赖直接运行。
   - 负责控制系统命令（`gps-sdr-sim` 和 `hackrf_transfer`），使用 Go 的 `exec` 包安全地管理进程生命周期，并使用 **Server-Sent Events (SSE)** 实现前端实时日志流推送。
   - 自动星历管理器：支持从 NOAA CORS (AWS S3) 自动下载最新的 GPS 星历（`brdc` 文件），并自动解压提供给模拟器使用。

2. **前端界面 (Vanilla HTML/CSS/JS + Leaflet Map)**:
   - **设计美学**：采用极具科技感的**暗黑玻璃微影风格 (Dark Glassmorphic UI)**，加入平滑渐变、脉冲式状态指示灯和流畅的微动画。
   - **交互地图**：使用开源的 **Leaflet.js**（配合 CartoDB Dark Matter 炫酷暗黑地图瓦片），支持在地图上直接点击选点获取经纬度与海拔。
   - **地址搜索**：集成免费免 Key 的 **OpenStreetMap Nominatim API**，实现全球地址搜索自动定位。
   - **实时控制台**：内置一个模拟的 Web Terminal，通过 SSE 实时接收 `gps-sdr-sim` 与 `hackrf_transfer` 的输出日志，便于调试和监控。

---

## 目录结构设计

```
/Users/work/hackrf-toys/
├── README.md               # 部署与配置指南
├── go.mod                  # Go 模块配置文件
├── main.go                 # Go Web 服务器入口（包含 API 接口与进程管理）
├── static/                 # 前端静态资源目录 (打包进 Go 二进制)
│   ├── index.html          # 控制台主页面
│   ├── app.js              # 地图、搜索、API 控制逻辑
│   └── app.css             # 暗黑玻璃微影风格样式表
└── data/                   # 数据与运行时目录
    ├── ephemeris/          # 星历存储目录
    │   └── brdc1350.26n    # 内置默认备用星历 (2026年5月15日)
    └── gps.bin             # 生成的 GPS IQ 临时数据文件 (发射完毕后自动清理)
```

---

## 功能模块与接口设计

### 1. 核心 API 接口

*   `GET /api/status`: 获取系统状态。
    *   返回：当前状态（`idle` / `generating` / `transmitting` / `error`）、是否有 HackRF 设备连接、**外置 10MHz TCXO 时钟源检测状态（通过 `hackrf_clock -i` 获取）**、当前运行的仿真参数、已缓存的星历文件列表。
    *   *时钟检测说明*：由于 HackRF 仅在开始射频传输/接收时才会激活外部时钟检测，后端在未传输时会显示默认提示，而在发射中会实时通过 `hackrf_clock -i` 读取 CLKIN 状态（若检测到将显示 `CLKIN status: clock signal detected`）。
*   `POST /api/simulation/start`: 启动 GPS 模拟。
    *   参数：`lat` (纬度), `lng` (经度), `alt` (海拔), `duration` (时长，秒), `gain` (增益，0-47), `ephemeris` (星历文件名)。
    *   逻辑：在后台异步执行 `gps-sdr-sim` 生成 `gps.bin`，生成完毕后自动启动 `hackrf_transfer` 进行发射。
*   `POST /api/simulation/stop`: 停止 GPS 模拟。
    *   逻辑：杀掉当前的 `hackrf_transfer` 或 `gps-sdr-sim` 进程，恢复系统为 `idle` 状态。
*   `GET /api/simulation/logs`: **SSE (Server-Sent Events) 流式日志接口**。
    *   逻辑：将后台进程的 stdout/stderr 实时以流的形式推送到前端控制台展示。
*   `POST /api/ephemeris/download`: 一键下载最新星历。
    *   逻辑：根据当前 UTC 日期，计算 NOAA CORS 的星历 URL，并后台自动下载、解压、校验，保存到 `data/ephemeris/` 下。
*   `POST /api/ephemeris/upload`: 手动上传星历。
    *   逻辑：支持用户手动上传本地的 `.n` 或 `.brdc` 星历文件。

### 2. 星历自动下载逻辑
*   基础 URL 格式：`https://geodesy.noaa.gov/corsdata/rinex/{YYYY}/{DDD}/brdc{DDD}0.{YY}n.gz`
*   其中 `{YYYY}` 为 4 位年份，`{DDD}` 为一年的第几天，`{YY}` 为两位年份。
*   Go 后端将尝试获取今天或昨天的星历（因为当天的星历可能有延迟发布），下载后使用 `gzip` 自动解压并保存。

### 3. 系统依赖管理
*   为了在 Ubuntu Server 上完美运行，我们将在 Go 中集成一个 `/api/system/setup` 接口或在启动时检查：
    - `hackrf_transfer` 是否在系统 PATH 中。
    - `gps-sdr-sim` 编译程序是否可用。如果不可用，支持一键在后台克隆并自动编译。

---

## 拟修改与新增文件列表

### [NEW] [go.mod](file:///Users/work/hackrf-toys/go.mod)
初始化 Go Module。

### [NEW] [main.go](file:///Users/work/hackrf-toys/main.go)
编写 Go 后端服务，包括：
- 进程管理器（Process Manager）：安全控制外部命令的启动和终止，捕获输出管道。
- 星历管理器（Ephemeris Manager）：下载、解压、缓存管理。
- HTTP 路由与嵌入式静态文件服务。
- SSE 实时日志转发服务。

### [NEW] [static/index.html](file:///Users/work/hackrf-toys/static/index.html)
设计主页面，包含：
- 地图容器。
- 搜索框与配置面板。
- 炫酷的状态大卡片、星历管理卡片、控制台 Web Terminal 卡片。
- 合规发射声明与 HackRF 连接说明。

### [NEW] [static/app.css](file:///Users/work/hackrf-toys/static/app.css)
实现高级视觉设计：
- 全暗黑风格 `#0f1015`，加入 `#1a1c24` 暗玻璃卡片，辅以青色（Cyan）、亮绿（Emerald）和警示红（Crimson）作为状态色彩。
- 半透明模糊滤镜 `backdrop-filter: blur(12px)`。
- 发射状态下状态灯的呼吸效果。

### [NEW] [static/app.js](file:///Users/work/hackrf-toys/static/app.js)
前端业务逻辑：
- Leaflet 地图初始化、深色底图图层加载。
- 鼠标点击地图选点获取坐标，拖拽标记实时同步。
- 搜索框触发 Nominatim 地址解析，实现无缝平移定位。
- SSE 连接建立与日志实时滚屏展示。
- API 交互：开始、停止、下载最新星历、上传星历、系统安装。

---

## 验证与测试方案

### 1. 自动化与单元测试
- 在 Go 中编写针对星历下载解析、路径生成的测试。
- 跨平台编译测试：在 Mac 上编译成 Linux 二进制，验证能够正常运行。
  `GOOS=linux GOARCH=amd64 go build -o gps-simulator-linux`

### 2. 手动功能验证 (Ubuntu Server 部署)
1. 将编译好的二进制及内置星历传输到 Ubuntu Server。
2. 运行 `./gps-simulator` 启动服务。
3. 在浏览器访问 `http://<ubuntu-server-ip>:8080`，测试：
   - 搜索地址如“天安门广场”，地图是否成功平移并精确定位。
   - 点击“下载最新星历”，是否成功从 NOAA 下载并解压。
   - 输入发射增益 30dB、发射时长 120 秒，点击“启动 GPS 模拟”。
   - 查看控制台是否实时输出 `gps-sdr-sim` 的生成进度 (1% -> 100%)。
   - 生成完毕后，是否成功启动 `hackrf_transfer`，且 HackRF One 上的 TX 指示灯亮起。
   - 点击“停止模拟”，进程是否被干净利落地杀掉，TX 指示灯熄灭，系统状态恢复 idle。
