# 🛰️ GPS Signal Simulator (基于 HackRF One 的 GPS 信号模拟测试工具)

这是一个专为 **HackRF One** 设备设计的、全面兼容 **Ubuntu Server 与 Windows 11** 测试环境的、**高颜值、极简部署**的 GPS 信号模拟测试工具。

系统采用 **Go 语言 + 嵌入式 Web 网页** 设计，可以编译成单个二进制可执行程序。您只需在 Ubuntu Server 上运行此单文件，即可在局域网内用任何设备的浏览器访问管理后台，完成“地图选点 ➡️ 自动下载星历 ➡️ 自动生成基带 ➡️ 一键发射 GPS 信号”的全生命周期控制。

---

## ✨ 核心特性

*   **单文件零依赖部署**：后端使用纯 Go 标准库开发，无任何第三方包依赖。使用 `go:embed` 将前端页面完全打包进 Go 二进制文件。
*   **高颜值暗黑玻璃微影 UI**：主控制台界面极具科技感，加入平滑渐变、呼吸式状态指示灯和流畅的微动画。
*   **交互式地图选点与地址搜索**：
    *   集成 **Leaflet.js**，并采用自定义 CSS 滤镜将地图呈现出绝美的黑客帝国式暗色调。
    *   支持地图任意位置点击或拖拽标记获取经纬度与海拔。
    *   集成免费免密匙的 **OpenStreetMap Nominatim API**，支持输入全球中文/英文地址直接定位。
*   **实时 SSE 日志控制台**：在网页端实现高保真 Linux Terminal，通过 **Server-Sent Events (SSE)** 流式推送后台 `gps-sdr-sim` 和 `hackrf_transfer` 进程的实时执行输出，秒级反馈。
*   **外置 10MHz TCXO 时钟源智能监测**：发射时自动调用 `hackrf_clock -i` 检测 CLKIN 时钟状态，精准显示您的 HackRF One 是否处于高精度锁定状态。
*   **混合星历管理**：
    *   **内置备用星历**：预装 2026 年 5 月 15 日的 GPS 星历文件，确保完全离线环境下即开即用。
    *   **一键拉取最新星历**：提供自动管理器，一键从 NOAA CORS (AWS S3 镜像) 下载最新的 GPS 星历并解压。
    *   **手动上传**：支持用户直接在网页拖拽或选择本地的 `.n` 或 `.brdc` 星历文件上传。
*   **一键式环境自动配置**：未配置环境的 Ubuntu 系统可点击网页一键触发后台 `apt-get` 依赖安装，克隆 `gps-sdr-sim` 并本地编译，全程自动部署。
*   **灵活的命令行启动参数**：支持自定义监听地址 (`-host`)、端口 (`-port`)、安全访问 Token (`-token`) 以及精美的 `--help` 帮助菜单。
*   **安全访问 Token 认证**：启用 `-token` 后，所有 API 请求需携带 Bearer Token 凭证。前端自动弹出高颜值毛玻璃登录弹窗，认证通过后 Token 持久化至浏览器 `localStorage`，后续操作自动携带。

---

## 🛠️ 系统要求与依赖

### 硬件要求
1.  **HackRF One SDR 平台**。
2.  **外置 10MHz TCXO 时钟模块**（推荐，GPS 信号对频率偏差极其敏感，无高精度时钟源接收机很难锁星）。
3.  **连接线缆**：建议采用 SMA 物理同轴线 + 射频衰减器（-30dB 至 -40dB）直接连接至您的 GPS 接收机（如手机或 GPS 模块天线口）。

### 软件环境 (Ubuntu Server)
系统运行需要以下组件（可通过网页“一键编译并配置环境”自动完成安装，也可手动执行）：
```bash
sudo apt update
sudo apt install -y git build-essential libfftw3-dev hackrf
```

---

## 🚀 编译与部署

本项目支持跨平台编译，可生成适用于 **Ubuntu Server (Linux)** 和 **Windows 11** 的单文件便携式程序。

> [!TIP]
> **Windows 11 用户快速指引**：如果您使用的是 Windows 11 环境，请阅读专用的 **[README_Windows.md](README_Windows.md)** 获取 Zadig 驱动替换及 PothosSDR 工具链的完整配置指南。

### 📦 1. 一键本地编译与便携化打包

为了方便您快速打包发布，项目在根目录下提供了自动化的本地打包脚本：

*   **在 macOS / Linux 下打包**（支持交叉编译出 Linux 与 Windows amd64 便携包）：
    ```bash
    # 授予执行权限并运行打包脚本
    chmod +x build_package.sh
    ./build_package.sh all
    ```
    *运行结束后，打包好的便携文件将输出在 `dist/` 目录下：*
    - `dist/gps-simulator-linux-amd64.tar.gz` (适用于 Ubuntu)
    - `dist/gps-simulator-windows-amd64.zip` (适用于 Windows 11)

*   **在 Windows 11 本地打包**（使用 PowerShell）：
    ```powershell
    # 运行 PowerShell 打包脚本
    .\build_package.ps1
    ```
    *同样会在 `dist/` 目录下生成上述两套平台便携包。*

---

### 2. 本地调试与手动编译

若您不需要完整打包，只需快速运行调试，可以执行标准 Go 命令：

#### macOS / Linux 调试运行
```bash
go build -o gps-simulator main.go
./gps-simulator
```

#### Windows 11 调试运行
```powershell
go build -o gps-simulator.exe main.go
.\gps-simulator.exe
```

#### 手动 Linux 交叉编译 (非打包)
```bash
GOOS=linux GOARCH=amd64 go build -o gps-simulator-linux main.go
```

---

### 3. Linux 部署与上传
使用 `scp` 或您常用的传输工具将打包好的 `gps-simulator-linux-amd64.tar.gz` 上传到您的 Ubuntu Server，并解压运行：
```bash
# 上传到服务器
scp dist/gps-simulator-linux-amd64.tar.gz user@your-ubuntu-ip:~/
```

### 4. 在 Ubuntu Server 上启动运行
SSH 登录您的 Ubuntu Server，进入对应目录，授予执行权限并启动：
```bash
chmod +x gps-simulator-linux
./gps-simulator-linux
```

#### 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-host` | string | `0.0.0.0` | Web 服务绑定的网卡 IP 地址。默认监听所有接口，适合局域网远程访问 |
| `-port` | int | `8080` | Web 服务监听端口 |
| `-token` | string | _(空，不启用)_ | 安全访问 Token。设置后所有 API 请求必须携带此凭证才能操作 |
| `-h` / `--help` | — | — | 显示定制化的帮助菜单 |

#### 启动示例

```bash
# 默认启动（局域网开放，无认证，端口 8080）
./gps-simulator-linux

# 仅监听本机回环地址，端口 9090
./gps-simulator-linux -host 127.0.0.1 -port 9090

# 启用安全 Token 认证
./gps-simulator-linux -token MySecretKey123

# 完整配置示例
./gps-simulator-linux -host 0.0.0.0 -port 8080 -token YourAccessToken
```

> **提示**：如果想让服务在后台持续运行，可以使用 `nohup` 或 `screen` / `systemd` 配置：
> `nohup ./gps-simulator-linux -token YourToken > server.log 2>&1 &`

打开您电脑的浏览器，输入：`http://<您的 Ubuntu 服务器 IP>:8080` 即可开始使用！

---

## 🤖 GitHub Actions 自动化 CI/CD

本项目已完整配置 **GitHub Actions** 自动化工作流。当您将代码推送至 GitHub 仓库后，系统将自动触发持续集成与构建：

1. **自动构建与测试（Push 到 `main` 分支时）**：
   - 每次有代码 push 或 PR 合并到 `main` 分支时，工作流会自动编译 Linux 与 Windows 版本的程序，并以 **Workflow Artifacts** 的形式发布。
   - 您可以直接在 GitHub Actions 运行记录页面下载最新测试版的 `gps-simulator-packages` 压缩包。
2. **自动发布版本（Push 版本标签 `v*` 时）**：
   - 当您在本地为代码打上版本标签并推送至 GitHub（例如 `git tag v1.0.0` 且 `git push origin v1.0.0`）时，GitHub Actions 将自动执行生产打包。
   - 并在 GitHub 仓库中**自动创建 Release 页面**，同时将 `gps-simulator-linux-amd64.tar.gz` 和 `gps-simulator-windows-amd64.zip` 作为 Release 附件上传供全球用户下载。

工作流配置文件位于：[.github/workflows/release.yml](.github/workflows/release.yml)。

---

## 📡 使用说明

### 🔐 安全访问认证（启用 Token 时）

如果启动时通过 `-token` 参数启用了安全认证，打开网页后会自动弹出毛玻璃 Token 认证弹窗：

1.  在弹窗中输入启动时设定的 Token 凭证，点击 **"验证并进入控制台"** 或按 `Enter` 键。
2.  验证通过后，Token 会自动保存到浏览器的 `localStorage` 中，后续刷新页面或发起 API 请求时将自动附带认证信息。
3.  如果 Token 错误，弹窗会显示红色错误提示并触发抖动动画，可重新输入。
4.  支持点击眼睛图标切换 Token 输入的明文/密文显示。

> **技术细节**：前端所有 `fetch()` API 调用通过 `Authorization: Bearer <token>` 头传递凭证；SSE 实时日志流（`EventSource`）因原生不支持自定义 Header，采用 URL 查询参数 `?token=<token>` 方式传递。

### 🗺️ 操作流程

1.  **确定测试坐标**：
    *   在地图上拖动蓝色标记，或者直接在地图上任意点击，坐标输入框将自动更新。
    *   在顶部的搜索栏中输入中文或英文地名（如"北京市天安门广场"），点击搜索，并选择联想出来的正确选项，地图会自动平移并精准标记。
2.  **星历准备**：
    *   如果您服务器处于联网状态，点击 **"自动拉取今日星历"** 按钮，系统会尝试自动下载最新星历并解压缩。
    *   也可以点击 **"手动上传星历"** 选择您本地的星历文件。
    *   如果两者都不可用，可以选择下拉列表中内置的 `brdc1350.26n` (虽然生成的时间在过去，但仍能有效用于测试定位锁星)。
3.  **发射配置**：
    *   设置需要发射的"持续时长"（默认 300 秒，基带数据大小约为 1.45GB，生成速度极快）。
    *   设置"发射增益"，默认 30dB（建议视衰减器配置大小进行微调，避免溢出与强干扰）。
4.  **一键发射**：
    *   点击亮绿色的 **"启动 GPS 信号模拟"** 按钮。
    *   可以在控制台终端日志中看到生成进度：`0%...50%...100%`。
    *   基带生成完毕后，会自动启动 HackRF One 进行射频发射，此时页面右上角 HackRF 状态将变为"发射中"，如果是高精度外置时钟，TCXO 状态将显示为"已锁定 (10MHz TCXO)"。
    *   测试过程中，您可以随时点击 **"强制终止发射"** 安全退出。

---

## ⚠️ 法律与合规警示 (Crucial Warning)

**GPS 信号仿真和欺骗（GPS Spoofing）在绝大多数国家和地区都受到极严格的无线电法规监管。**

*   **严禁** 在露天环境通过天线向空中发射模拟信号。这会干扰真实的 GPS 导航系统（如民航、车载导航、紧急定位服务），造成灾难性后果并承担刑事法律责任。
*   **必须** 在完全屏蔽的实验室环境（如微波暗室、法拉第电笼）内，使用天线进行小范围无线测试。
*   **或者** 采用有线回环方式测试：使用同轴电缆物理连接 HackRF 的 TX 口与接收机的天线输入口，并在中间必须接入 `-30dB` 至 `-40dB` 以上的**射频衰减器**，以防止大功率信号烧毁接收机射频前端。

**因不当操作或违规发射引起的任何法律纠纷及硬件损坏，需由使用者本人完全承担。**
