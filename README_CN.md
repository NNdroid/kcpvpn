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
- 🎭 **五大反 DPI 流量伪装协议 (`-obfs`)**:
  - `stun`: **WebRTC 实时音视频伪装 (RFC 5389)**，伪装为 Discord / Google Meet / Zoom 通话流量。
  - `dns`: **DNS 域名查询伪装 (RFC 1035)**，包装为标准 UDP DNS 53 查询与响应。
  - `dtls`: **DTLS 1.2 加密安全传输伪装**，伪装为 Cisco AnyConnect / OpenVPN 流量。
  - `quic`: **HTTP/3 QUIC 网页流量伪装**，伪装为 Cloudflare / Google HTTP/3 网页流量。
  - `xor`: **动态字节掩码混淆**，基于 PSK 派生掩码彻底模糊数据帧边界。
- 🎲 **动态随机数据包长度填充 (`-pad-max`)**:
  - 发包时在尾部随机附加 0 ~ N 字节加密噪点，打乱流量数据包长度统计直方图，防御机器学习/包长特征识别。
- 🔀 **二层 Ethernet 交换与动态 IPAM 地址池**:
  - 内置以太网 MAC 学习表与广播/单播转发。
  - 自动解析 IPv4 & IPv6 CIDR 网段，支持动态下发客户端 IP、网关与子网掩码，支持特定 IP 保留。
- 🛡️ **前向纠错 (FEC) 与强加密**:
  - 支持 Reed-Solomon FEC 纠错（可配置数据片与校验片比例）。
  - 支持 AES 与 Salsa20 加密。
- 🎛️ **QoS 服务质量与传输调优**:
  - 支持 `-dscp` QoS 报文优先级标记（如 `DSCP 46` 专属低延迟通道）。
  - 支持 `-mtu` (默认 1350，防公网 IP 分片)、`-sndwnd` / `-rcvwnd` 窗口调节。
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
> **注意**: 服务端与客户端必须使用相同的预共享密钥 (`-psk`)、伪装模式 (`-obfs`) 和填充参数 (`-pad-max`)。运行需要管理员/root 权限以操作 TAP 虚拟网卡。

### 1. 启动服务端 (Server Mode)

在 Linux 服务器（如 Ubuntu / Debian / CentOS）上运行：

```bash
# 启动服务端 (开启 STUN 音视频伪装 + 128 字节随机填充 + DSCP 46 优先级)
sudo ./kcpvpn_linux_amd64 \
  -mode server \
  -addr 0.0.0.0:4000 \
  -psk "YourSuperSecretPassword" \
  -v4net "10.8.0.0/24" \
  -obfs stun \
  -pad-max 128 \
  -dscp 46
```

### 2. 启动客户端 (Client Mode)

#### 🪟 Windows 客户端:
使用管理员身份打开 PowerShell 或 CMD 运行：

```cmd
kcpvpn_windows_amd64.exe ^
  -mode client ^
  -addr "YOUR_SERVER_IP:4000" ^
  -psk "YourSuperSecretPassword" ^
  -obfs stun ^
  -pad-max 128
```
*(注：Windows 需安装标准 [OpenVPN TAP-Windows 驱动](https://build.openvpn.net/downloads/releases/)；程序会自动智能匹配已安装的 TAP 适配器。)*

#### 🐧 Linux 客户端:
```bash
sudo ./kcpvpn_linux_amd64 \
  -mode client \
  -addr "YOUR_SERVER_IP:4000" \
  -psk "YourSuperSecretPassword" \
  -obfs stun \
  -pad-max 128
```

#### 🍎 macOS 客户端:
```bash
sudo ./kcpvpn_darwin_arm64 \
  -mode client \
  -addr "YOUR_SERVER_IP:4000" \
  -psk "YourSuperSecretPassword" \
  -obfs stun \
  -pad-max 128
```

---

## 📋 命令行参数详解 (CLI Reference)

| Flag 参数 | 默认值 | 适用模式 | 描述 |
| :--- | :--- | :--- | :--- |
| `-mode` | `server` | 全部 | 运行模式: `server` (服务端) 或 `client` (客户端) |
| `-addr` | `127.0.0.1:4000` | 全部 | 监听/连接的 UDP Socket 地址 |
| `-psk` | `HereIsYour...` | 全部 | 预共享密钥 (Pre-Shared Key) |
| `-obfs` | `none` | 全部 | 流量伪装模式: `none`, `stun`, `dns`, `dtls`, `quic`, `xor` |
| `-pad-max` | `0` | 全部 | 报文最大随机填充字节数 (如 `128`，`0` 表示禁用) |
| `-crypt` | `salsa20` | 全部 | KCP 传输加密算法: `salsa20`, `aes`, `none` |
| `-mtu` | `1350` | 全部 | KCP 最大传输单元 (建议 1350 避免公网分片) |
| `-sndwnd` | `4096` | 全部 | KCP 发送滑动窗口大小 |
| `-rcvwnd` | `4096` | 全部 | KCP 接收滑动窗口大小 |
| `-fec-d` | `5` | 全部 | Reed-Solomon FEC 数据分片数 |
| `-fec-p` | `2` | 全部 | Reed-Solomon FEC 校验分片数 |
| `-dscp` | `0` | 全部 | IP DSCP QoS 报文优先级标记 (如 `46` 最高低延迟队列) |
| `-tap` | `tap0` | 全部 | TAP 虚拟网卡设备名称 |
| `-v4net` | `10.0.0.0/24` | 服务端 | 服务端 IPv4 CIDR 虚拟子网 |
| `-v6net` | `fd00::/64` | 服务端 | 服务端 IPv6 CIDR 虚拟子网 |
| `-ip4` | `""` | 客户端 | 客户端静态请求分配的指定 IPv4 |
| `-fwmark` | `0` | 客户端 | 策略路由 fwmark 标记 (仅 Linux 支持，例如 `1911`) |
| `-loglevel`| `warn` | 全部 | 日志级别: `debug`, `info`, `warn`, `error` |
| `-v` | `false` | 全部 | 显示当前版本号并退出 |

---

## 📦 数据包结构图 (Packet Structure)

```text
+---------------------------------------------------------------------------------------+
| 1. 外层网络传输头 (Outer IP & UDP Headers)                                             |
|    ├── IP Header (20 字节, 包含 -dscp 设置的 QoS 标记)                                  |
|    └── UDP Header (8 字节, 包含源/目的 UDP 端口)                                      |
+---------------------------------------------------------------------------------------+
| 2. KCP 协议层与加密/FEC纠错层 (KCP ARQ & Crypto Layer)                                  |
|    ├── KCP Header (24 字节: Conv, Cmd, Frg, Wnd, TS, Sn, Una, Len)                    |
|    └── 加密保护 (AES / Salsa20 块加密，加密以下所有载荷)                                |
+=======================================================================================+
| 3. 流量伪装头 (Obfuscation Header) -- 由 -obfs 参数控制                                  |
|    ├── obfs=stun : STUN 报文头 (20 字节: 包含 Magic Cookie 0x2112A442 & Transaction ID) |
|    ├── obfs=dns  : DNS 标头  (12 字节: 包含 Transaction ID, Flags, QD/AN Count)       |
|    ├── obfs=dtls : DTLS Record头 (13 字节: ContentType 0x17, Version 0xFEFF, Epoch, Seq)|
|    ├── obfs=quic : QUIC Short Header (8 字节: Connection ID & Packet Number)         |
|    ├── obfs=xor  : 无附加头，直接对后续数据实施 PSK 派生掩码 XOR 混淆                    |
|    └── obfs=none : 无伪装头 (0 字节)                                                  |
+---------------------------------------------------------------------------------------+
| 4. 内部数据帧头 (Inner Frame Header - 3 字节)                                          |
|    ├── 载荷长度 (Payload Length): 2 字节 (Big-Endian uint16)                           |
|    └── 填充长度 (Padding Length p): 1 字节 (uint8, 值为 0 ~ pad-max)                    |
+---------------------------------------------------------------------------------------+
| 5. 真实二层以太网帧 (Raw Ethernet Frame) -- 长度等于 Payload Length                     |
|    ├── 目的 MAC 地址 (Destination MAC): 6 字节                                        |
|    ├── 源 MAC 地址 (Source MAC): 6 字节                                               |
|    ├── 以太网类型 (EtherType): 2 字节 (0x0800 IPv4 / 0x0806 ARP / 0x86DD IPv6)         |
|    └── IP / ARP 数据包 Payload (如内网 TCP/UDP 数据)                                  |
+---------------------------------------------------------------------------------------+
| 6. 动态随机填充尾 (Dynamic Padding Tail) -- 长度等于 Padding Length (p)                 |
|    └── 加密随机噪点字节 (p 字节，由密码学随机数生成器产生)                              |
+---------------------------------------------------------------------------------------+
```

---

## 📜 许可证 (License)

本项目采用 [GPL-3.0 许可证](LICENSE)。
