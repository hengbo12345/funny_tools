# 🛰️ GPS 信号模拟测试工具 - Windows 11 部署与使用手册

本手册专门针对 **Windows 11** 环境，指导您如何完成 HackRF 硬件驱动安装、SDR 工具链部署以及启动 GPS 信号模拟测试工具。

---

## 🛠️ 第一步：安装 HackRF 硬件驱动 (WinUSB)

Windows 默认无法识别 HackRF One 设备作为通用 SDR 硬件。您需要使用 **Zadig** 工具将驱动替换为 **WinUSB**：

1. **连接硬件**：通过 USB 线将 HackRF One 连接至您的 Windows 11 电脑。
2. **下载 Zadig**：访问 [Zadig 官网](https://zadig.akeo.ie/) 并下载最新版本的 Zadig 软件（免安装，直接运行）。
3. **选择设备**：
   - 打开 Zadig，点击菜单栏的 **Options** ➡️ 勾选 **List All Devices**。
   - 在中间的下拉列表中选择 **HackRF One**（如果显示为 `Interface 0`，确保它是 HackRF 对应的接口）。
4. **安装驱动**：
   - 确认右侧的 Target Driver 框中显示为 **WinUSB (v6.1.7600.16385 或更高)**。
   - 点击 **Replace Driver**（或 **Install Driver** / **Reinstall Driver**）按钮。
   - 等待几分钟，直到提示驱动替换成功（Driver installed successfully）。

---

## ⚙️ 第二步：部署 Windows SDR 工具链 (PothosSDR)

SDR 工具链中包含了必不可少的命令行程序，如 `hackrf_transfer.exe` 和 `hackrf_info.exe`：

### 方法 A：使用 Chocolatey 一键安装 (推荐)
如果您安装了 Windows 包管理器 Chocolatey，在管理员模式的 PowerShell 中运行：
```powershell
choco install pothossdr
```

### 方法 B：手动下载安装
1. 访问 [PothosSDR GitHub Releases](https://github.com/pothosware/PothosSDR/releases) 下载最新的 Windows 安装包（如 `.exe` 格式）。
2. 运行安装程序，在安装过程中**勾选“将 PothosSDR 目录添加至系统 PATH 环境变量”**（Add PothosSDR to system PATH）。
3. 安装完成后，打开一个新的 CMD 或 PowerShell 终端，运行以下命令验证安装：
   ```cmd
   hackrf_info
   ```
   *如果驱动和工具链均配置正确，将输出包含 `Found HackRF` 和您的设备序列号等信息。*

---

## 📡 第三步：获取并部署 `gps-sdr-sim.exe`

本工具的信号基带生成依赖于 `gps-sdr-sim`。在 Windows 上您需要获取其编译好的 `.exe` 文件：

1. **直接下载预编译二进制文件**：
   - 访问 [gps-sdr-sim 官方 GitHub Releases](https://github.com/osqzss/gps-sdr-sim/releases)。
   - 下载适用于 Windows 的最新 `gps-sdr-sim.exe` 压缩包并解压。
2. **部署文件**：
   - 将解压缩得到的 `gps-sdr-sim.exe` 放置在与本工具的 **`gps-simulator.exe` 同一根目录下**。
   - 结构类似于：
     ```
     /你的解压目录/
     ├── gps-simulator.exe   # 本 Web 服务主程序
     ├── gps-sdr-sim.exe     # 基带生成依赖程序
     ├── README.md
     └── data/               # 星历数据目录
         └── ephemeris/
             └── brdc1350.26n
     ```

> **提示**：如果您想在 Windows 上本地编译 `gps-sdr-sim.exe`，您可以使用 **MSYS2** 或 **MinGW**，在 `gps-sdr-sim` 源码目录下运行：`gcc gps-sdr-sim.c -O3 -lm -o gps-sdr-sim.exe`。

---

## 🚀 第四步：启动与运行

您可以直接双击运行 `gps-simulator.exe`，或者在命令提示符 (CMD) 或 PowerShell 中启动，以配置高级参数：

```powershell
# 1. 进入程序所在目录
cd C:\path\to\your-folder

# 2. 默认启动 (局域网可访问，无密码认证)
.\gps-simulator.exe

# 3. 启用安全 Token 认证并指定端口
.\gps-simulator.exe -port 8080 -token YourSecurePassword
```

### 访问网页
打开浏览器访问：`http://localhost:8080` (如果您设置了 `-token`，请在弹窗中输入刚才启动时设置的 Token)。

---

## ❓ 常见问题与排错 (FAQ)

### 1. 网页后台日志提示 `未检测到连接的 HackRF One 设备`？
*   请确认 USB 线是否插紧，且状态灯已亮起。
*   确认您是否已成功通过 Zadig 替换了 WinUSB 驱动。
*   检查任务管理器或后台，确保没有其他 SDR 软件 (如 SDR#, CubicSDR, GNU Radio) 正在独占占用您的 HackRF。

### 2. 网页后台日志提示 `安装依赖`，点击一键配置有用吗？
*   在 Windows 平台下，“一键配置环境”功能被设计为优雅退出，并会直接在网页终端日志中为您打印出这篇快速安装指南。Windows 上**无法通过程序自动执行一键编译**，您必须按照此手册手动放置 `gps-sdr-sim.exe` 和安装 PothosSDR。

### 3. 点击“启动 GPS 信号模拟”后，手机/GPS 接收机没有反应或无法搜星？
*   **时钟源问题**：GPS 信号对频率偏差极其敏感。**强烈建议**在 HackRF One 上加装外置 **10MHz TCXO 高精度温补时钟源**。若无高精度时钟源，因板载晶振温漂较大，接收机很难锁星。
*   **衰减器配置**：Windows 本地发射时，请务必在 HackRF 的 TX 发射天线接口与被测设备天线接口之间接入 **-30dB 至 -40dB 的物理同轴射频衰减器**。不要露天发射，防止大功率无线电干扰和烧毁接收机。
*   **星历过期**：若您使用的星历太旧，设备可以锁星，但手机的真实定位时间戳可能无法完全同步。点击网页上的“自动拉取今日星历”获取当日最新星历可完美解决。
