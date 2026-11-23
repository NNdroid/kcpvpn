# KCPVPN (中文文档)

语言: [English](README.md) | [中文 README](README_CN.md)

🚀 **KCPVPN** 是一款基于 KCP 协议的高性能、低延迟、抗 DPI 封锁的跨平台二层 TAP 虚拟专用网络（VPN）系统。

内置**多协议流量伪装（STUN / DNS / DTLS / QUIC / XOR）**、**动态数据包长度随机填充**、**Reed-Solomon 前向纠错 (FEC)**、**二层虚拟交换机 (VSwitch) 双重检查锁并发优化**，并支持 **Linux、Windows 与 macOS** 系统的全自动化构建与配置。

---

## 🌟 核心特性

- 🌐 **全平台支持**: 原生支持 **Linux**、**Windows** (TAP-Windows 智能适配) 和 **macOS** (`tap` / `utun`)。
- ⚡ **超低延迟与高吞吐**:
  - 底层基于 KCP ARQ 协议，优化乱序重传与丢包恢复。
  - 二层虚拟交换机 (VSwitch) 实现**读锁优先双重检查锁定 (Double-Checked Locking)**，消除高并发数据包转发时的全局写锁争用。
  - 全链路 `sync.Pool` 内存池复用，极大降低 GC 延迟与 CPU 开销。
- 🎭 **五大反 DPI 流量伪装协议 (`obfs` 配置项)**:
  - `stun`: **WebRTC 实时音视频伪装 (RFC 5389)**，伪装为 Discord / Google Meet / Zoom 通话流量。
  - `dns`: **DNS 域名查询伪装 (RFC 1035)**，包装为标准 UDP DNS 53 查询与响应。
  - `dtls`: **DTLS 1.2 加密安全传输伪装**，伪装为 Cisco AnyConnect / OpenVPN 流量。
  - `quic`: **HTTP/3 QUIC 网页流量伪装**，伪装为 Cloudflare / Google HTTP/3 网页流量。
  - `xor`: **动态字节掩码混淆**，基于 PSK 派生掩码彻底模糊数据帧边界。
- 🎲 **动态随机数据包长度填充 (`pad-max` 配置项)**:
  - 发包时在尾部随机附加 0 ~ N 字节加密噪点，打乱流量数据包长度统计直方图，防御机器学习/包长特征识别。
- 🔀 **二层 Ethernet 交换与动态 IPAM 地址池**:
  - 内置以太网 MAC 学习表与广播/单播转发。
  - 自动解析 IPv4 & IPv6 CIDR 网段，支持动态下发客户端 IP、网关与子网掩码，支持特定 IP 保留。
- 🛡️ **前向纠错 (FEC) 与认证加密**:
  - 支持 Reed-Solomon FEC 纠错（可配置数据片与校验片比例）。
  - 默认 **AES-GCM 认证加密**（`-crypt aes-gcm`），同时支持 AES-CFB、Salsa20；密钥由 PSK 经 **PBKDF2**（40960 次迭代）派生，提升暴力破解成本。
- 🎛️ **QoS 服务质量与传输调优**:
  - 支持 `-dscp` QoS 报文优先级标记（如 `DSCP 46` 专属低延迟通道）。
  - `mtu` 表示**隧道内网卡 MTU**（默认 **1500**），KCP 外层数据报大小自动派生以**避免公网 IP 分片**；`sndwnd` / `rcvwnd` 窗口调节。
- 🔒 **连接安全**:
  - `psk` 预共享密钥**必填**（不再提供公开默认值）。
  - 服务端支持 `max-clients` 并发连接上限，拒绝超出部分的连接。
  - 二层交换对客户端帧强制**源 MAC == 注册 MAC** 校验，阻断 MAC 表污染与源地址仿冒。
- 🚀 **自动化 CI/CD 编译与 Release**:
  - 包含 Bash (`build.sh`) 与 PowerShell (`build.ps1`) 交叉编译脚本。
  - 内置 GitHub Actions 工作流，打 Tag 自动跨平台构建 11 种平台架构资产并发布 Release。

---

## 🛠️ 编译说明

### 本地编译

需要安装 **Go 1.22+** 环境：

```bash
# 克隆代码库
git clone https://github.com/NNdroid/kcpvpn.git
cd kcpvpn

# Linux / macOS / Git Bash 环境编译所有平台二进制
bash ./scripts/build.sh

# Windows PowerShell 环境编译
.\scripts\build.ps1
```

编译产物将存放在 `bin/` 目录下（如 `kcpvpn_linux_amd64`、`kcpvpn_windows_amd64.exe` 等）。

---

## 🚀 快速上手

> [!IMPORTANT]
> **注意**: 所有配置均通过 `config.json` 文件提供。**命令行只保留 `-version` 与 `-config`**。
> `psk` 预共享密钥**必填**；服务端与客户端必须使用相同的密钥、`obfs` 伪装模式和 `pad-max` 填充参数。
> 运行需要管理员/root 权限以操作 TAP 虚拟网卡。

### 0. 准备配置文件

```bash
cp config.example.json config.json
# 编辑 config.json，至少修改 "psk" 为强密钥；按需调整 mode/addr/v4net/obfs 等。
```

### 1. 启动服务端 (Server Mode)

```bash
# 启动服务端 (使用 config.json：开启 STUN 音视频伪装 + 128 字节随机填充 + DSCP 46 优先级)
sudo ./kcpvpn_linux_amd64 -config config.json
```

### 2. 启动客户端 (Client Mode)

#### 🪟 Windows 客户端:
使用管理员身份打开 PowerShell 或 CMD 运行：
```cmd
kcpvpn_windows_amd64.exe -config config.json
```
*(注：Windows 需安装标准 [OpenVPN TAP-Windows 驱动](https://build.openvpn.net/downloads/releases/)；程序会自动智能匹配已安装的 TAP 适配器。)*

#### 🐧 Linux 客户端:
```bash
sudo ./kcpvpn_linux_amd64 -config config.json
```

#### 🍎 macOS 客户端:
```bash
sudo ./kcpvpn_darwin_arm64 -config config.json
```

---

## 📋 config.json 字段详解 (Configuration Reference)

> 未填写的字段自动使用默认值。`-config` 指定配置文件路径（默认 `config.json`）；`-version` 显示版本号后退出。

| 字段 | 默认值 | 适用模式 | 描述 |
| :--- | :--- | :--- | :--- |
| `mode` | `server` | 全部 | 运行模式: `server` (服务端) 或 `client` (客户端) |
| `addr` | `127.0.0.1:4000` | 全部 | 监听/连接的 UDP Socket 地址 |
| `tap` | `tap0` | 全部 | TAP 虚拟网卡设备名称 |
| `v4net` | `10.0.0.0/24` | 服务端 | 服务端 IPv4 CIDR 虚拟子网 |
| `v6net` | `fd00::/64` | 服务端 | 服务端 IPv6 CIDR 虚拟子网 |
| `fec-d` | `5` | 全部 | Reed-Solomon FEC 数据分片数（`0` 表示停用） |
| `fec-p` | `2` | 全部 | Reed-Solomon FEC 校验分片数（`0` 表示停用） |
| `crypt` | `aes-gcm` | 全部 | KCP 传输加密算法: `aes-gcm`, `salsa20`, `aes`, `none` |
| `psk` | *(必填)* | 全部 | 预共享密钥 (Pre-Shared Key)，由 PBKDF2 派生加密密钥 |
| `obfs` | `none` | 全部 | 流量伪装模式: `none`, `stun`, `dns`, `dtls`, `quic`, `xor` |
| `pad-max` | `0` | 全部 | 每个外层数据报最大随机填充字节数 (如 `128`，`0` 表示禁用) |
| `mtu` | `1500` | 全部 | **隧道内网卡 MTU**；KCP 外层数据报大小自动派生以避免公网分片 |
| `sndwnd` | `4096` | 全部 | KCP 发送滑动窗口大小 |
| `rcvwnd` | `4096` | 全部 | KCP 接收滑动窗口大小 |
| `dscp` | `0` | 全部 | IP DSCP QoS 报文优先级标记 (如 `46` 最高低延迟队列) |
| `log-level` | `warn` | 全部 | 日志级别: `debug`, `info`, `warn`, `error` |
| `max-clients` | `256` | 服务端 | 最大并发客户端连接数 |
| `max-rate` | `0` | 服务端 | 每连接客户端→服务端注入速率上限（字节/秒，`0`=不限） |
| `fwmark` | `0` | 客户端 | 策略路由 fwmark 标记 (仅 Linux 支持，例如 `1911`) |
| `ip4` | `""` | 客户端 | 客户端静态请求分配的指定 IPv4 |
| `ip6` | `""` | 客户端 | 客户端静态请求分配的指定 IPv6 |
| `mac` | `""` | 客户端 | 客户端请求指定 MAC（可选；留空则由服务端生成并下发） |
| `web-enabled` | `false` | 全部 | 是否启动 Web 管理界面 |
| `web-bind` | `local` | 全部 | Web UI 绑定模式: `tunnel`（仅 tunnel IP）、`local`（127.0.0.1/::1）、`public`（所有公网 IP） |
| `web-addr` | `:8080` | 全部 | Web UI 监听地址（端口来源，如 `:8080` 或 `127.0.0.1:8080`） |
| `web-token` | `""` | 全部 | Web UI 访问令牌（Bearer）。`web-bind=public` 时必须设置 |

> **命令行参数**: `-version` 显示版本号；`-config <path>` 指定配置文件（默认 `config.json`）。其余参数均已移除。

---

## 📦 数据包结构图 (Packet Structure)

> 伪装层位于 **KCP 之外**（包装每个 UDP 数据报），因此伪装头真实出现在线缆上，可被 DPI 识别为 STUN / DNS / DTLS / QUIC 等协议。

```text
+---------------------------------------------------------------------------------------+
| 1. 外层网络传输头 (Outer IP & UDP Headers)                                             |
|    ├── IP Header (20 字节, 包含 -dscp 设置的 QoS 标记)                                  |
|    └── UDP Header (8 字节, 包含源/目的 UDP 端口)                                      |
+---------------------------------------------------------------------------------------+
| 2. 流量伪装层 (Obfuscation Layer) -- 由 -obfs 参数控制，包装每个 KCP 数据报             |
|    ├── obfs=stun : STUN 报文头 (20 字节: 包含 Magic Cookie 0x2112A442 & Transaction ID) |
|    ├── obfs=dns  : DNS 标头  (12 字节: 包含 Transaction ID, Flags, QD/AN Count)       |
|    ├── obfs=dtls : DTLS Record头 (13 字节: ContentType 0x17, Version 0xFEFF, Epoch, Seq)|
|    ├── obfs=quic : QUIC Short Header (8 字节: Connection ID & Packet Number)         |
|    ├── obfs=xor  : 对整个数据报实施 PSK 派生掩码 XOR 混淆                              |
|    └── obfs=none : 无伪装头 (0 字节)                                                  |
|    └── 动态随机填充尾 (p 字节) -- 由 -pad-max 控制，打乱报文长度直方图                  |
+---------------------------------------------------------------------------------------+
| 3. KCP 协议层与认证加密/FEC 层 (KCP ARQ & Crypto Layer)                                |
|    ├── 随机 Nonce (加密头) + 认证标签 (AES-GCM)                                       |
|    ├── KCP Header (24 字节: Conv, Cmd, Frg, Wnd, TS, Sn, Una, Len)                    |
|    └── 加密保护 (AES-GCM / AES / Salsa20，加密以下所有载荷，AES-GCM 附带完整性认证)     |
+=======================================================================================+
| 4. 内部数据帧头 (Inner Frame Header - 3 字节)                                          |
|    ├── 帧类型 (Frame Type): 1 字节 (0x01=握手, 0x02=keepalive, 0x03=以太网数据)        |
|    └── 载荷长度 (Payload Length): 2 字节 (Big-Endian uint16)                           |
+---------------------------------------------------------------------------------------+
| 5. 真实二层以太网帧 (Raw Ethernet Frame) -- 长度等于 Payload Length                     |
|    ├── 目的 MAC 地址 (Destination MAC): 6 字节                                        |
|    ├── 源 MAC 地址 (Source MAC): 6 字节                                               |
|    ├── 以太网类型 (EtherType): 2 字节 (0x0800 IPv4 / 0x0806 ARP / 0x86DD IPv6)         |
|    └── IP / ARP 数据包 Payload (如内网 TCP/UDP 数据)                                  |
+---------------------------------------------------------------------------------------+
```

> **MTU 说明**：`-mtu 1500` 表示隧道内网卡 MTU 为 1500。KCP 外层数据报大小自动派生，保证外层 IP 包不超过 1500 字节以避免公网分片。由于以太网帧（1514 字节）+ 协议开销在物理 1500 链路下无法放入单个 KCP 段，完整 MTU 帧会由 KCP 拆分为 2 段传输（数学上不可避免，属架构最优值）。

> **安全说明**：MAC 由服务端在加密握手内生成并下发，客户端配置网卡使真实 MAC 与注册一致；服务端强制"客户端帧源 MAC == 注册 MAC"，客户端不再自报 MAC，杜绝仿冒。握手携带随机 Nonce + 时间戳，服务端校验时间窗（±300s）并去重，防止重放；握手后数据面只接受类型帧（0x03 数据 / 0x02 keepalive），违规断开。数据面按连接限速（`max-rate`），VSwitch 队列满时计数并限频告警。

---

## 🌐 Web 管理界面 (Web UI)

设置 `web-enabled: true` 启动内置 HTTP 管理界面（服务端与客户端均可用）。默认关闭。

**绑定模式 `web-bind`** 决定监听哪些 IP：

| 模式 | 监听地址 | 说明 |
| :--- | :--- | :--- |
| `local` | `127.0.0.1` + `::1` | 仅本机回环（默认，安全） |
| `tunnel` | 隧道网卡 IP | 服务端绑 `ServerV4`/`ServerV6`；客户端绑其分配的隧道 IP |
| `public` | `0.0.0.0` + `::` | 所有地址（含公网） |

**鉴权**：请求需携带 `Authorization: Bearer <web-token>`。`web-bind=public` 时**必须**设置 `web-token`，否则启动报错；`local`/`tunnel` 且 `web-token` 为空时放行（本机调试便利）。

**功能**（服务端）：查看在线客户端（IP/MAC/连接时长）、踢出客户端、查看/编辑配置；**客户端**：查看隧道状态（IP/MAC/网关/重连次数）、查看/编辑配置。保存配置后写入 `config.json` 并**自动重启进程**生效（跨平台：spawn 自身 + 退出旧进程；端口/TAP 释放由父子两侧延迟缓解）。`mode` 不可在线切换（需手动改配置重启）。

**端口来源**：`web-addr` 提供端口（如 `:8080`），实际监听 IP 由 `web-bind` 决定。

> **安全提示**：`web-bind=public` 会将管理界面暴露到公网，务必设置强 `web-token`。默认 `local` 仅供本机访问。

---

## 📜 许可证 (License)

本项目采用 [GPL-3.0 许可证](LICENSE)。
