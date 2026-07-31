# KCPVPN

Language: [English](README.md) | [中文 README](README_CN.md)

🚀 **KCPVPN** is a high-performance, ultra-low-latency, cross-platform Layer-2 TAP Virtual Private Network (VPN) system built on top of the KCP protocol.

It features **multi-protocol traffic camouflage (STUN / DNS / DTLS / QUIC / XOR)**, **dynamic random packet length padding**, **Reed-Solomon Forward Error Correction (FEC)**, **double-checked locking concurrency optimization in Layer-2 VSwitch**, and full cross-platform support for **Linux, Windows, and macOS**.

---

## 🌟 Key Features

- 🌐 **Cross-Platform Support**: Native support for **Linux**, **Windows** (smart TAP-Windows adapter matching), and **macOS** (`tap` / `utun`).
- ⚡ **Low Latency & High Throughput**:
  - Built on KCP ARQ protocol for fast out-of-order retransmission and packet loss recovery.
  - Layer-2 Virtual Switch (VSwitch) featuring **read-first Double-Checked Locking**, eliminating global write lock contention during high-throughput packet forwarding.
  - End-to-end `sync.Pool` buffer pooling, minimizing GC pauses and CPU overhead.
- 🎭 **Anti-DPI Multi-Protocol Camouflage (`-obfs`)**:
  - `stun`: **WebRTC Audio/Video Call Camouflage (RFC 5389)**, disguising traffic as Discord, Google Meet, or Zoom calls.
  - `dns`: **DNS Query Camouflage (RFC 1035)**, wrapping packets as standard UDP DNS 53 queries and responses.
  - `dtls`: **DTLS 1.2 Encrypted Record Camouflage**, disguising traffic as Cisco AnyConnect / OpenVPN SSL VPN.
  - `quic`: **HTTP/3 QUIC Header Camouflage**, disguising traffic as Cloudflare / Google HTTP/3 web traffic.
  - `xor`: **Dynamic Byte Masking**, obscuring packet boundaries with PSK-derived key masks.
- 🎲 **Dynamic Random Packet Padding (`-pad-max`)**:
  - Appends `0` to `N` random padding bytes at the end of each packet to distort length statistics and defeat DPI fingerprinting.
- 🔀 **Layer-2 Ethernet Switching & Dynamic IPAM**:
  - Built-in MAC address learning table with unicast and multicast packet forwarding.
  - Parses IPv4 & IPv6 CIDR subnets, dynamically distributing client IP, gateway, mask, and supporting static IP reservations.
- 🛡️ **Forward Error Correction (FEC) & Strong Encryption**:
  - Reed-Solomon FEC (configurable data & parity shards ratio).
  - AES-128 and Salsa20 block ciphers.
- 🎛️ **QoS & Transport Tuning**:
  - Supports `-dscp` QoS IP header tagging (e.g. `DSCP 46` for Expedited Low-Latency Queue).
  - Supports `-mtu` (default `1350` to avoid WAN fragmentation), `-sndwnd` / `-rcvwnd` window controls.
- 🚀 **Automated CI/CD Cross-Compilation & GitHub Releases**:
  - Cross-platform build scripts for Bash (`build.sh`) and PowerShell (`build.ps1`).
  - GitHub Actions workflow to automatically compile and release 11 platform architectures on tag push.

---

## 🛠️ Build Instructions

### Building Locally

Requires **Go 1.22+**:

```bash
# Clone repository
git clone https://github.com/NNdroid/kcpvpn.git
cd kcpvpn

# Compile binaries for all platforms on Linux / macOS / Git Bash
bash ./scripts/build.sh

# Compile on Windows PowerShell
.\scripts\build.ps1
```

Compiled binaries will be generated inside the `bin/` directory (`kcpvpn_linux_amd64`, `kcpvpn_windows_amd64.exe`, etc.).

---

## 🚀 Quick Start

> [!IMPORTANT]
> **Note**: Server and Client must use matching Pre-Shared Keys (`-psk`), obfuscation modes (`-obfs`), and padding limits (`-pad-max`). Root or Administrator privileges are required to configure TAP network interfaces.

### 1. Server Mode

On a Linux server (Ubuntu / Debian / CentOS):

```bash
# Start Server with STUN WebRTC camouflage + 128-byte random padding + DSCP 46 priority
sudo ./kcpvpn_linux_amd64 \
  -mode server \
  -addr 0.0.0.0:4000 \
  -psk "YourSuperSecretPassword" \
  -v4net "10.8.0.0/24" \
  -obfs stun \
  -pad-max 128 \
  -dscp 46
```

### 2. Client Mode

#### 🪟 Windows Client:
Open PowerShell or CMD as Administrator:

```cmd
kcpvpn_windows_amd64.exe ^
  -mode client ^
  -addr "YOUR_SERVER_IP:4000" ^
  -psk "YourSuperSecretPassword" ^
  -obfs stun ^
  -pad-max 128
```
*(Requires standard [OpenVPN TAP-Windows Driver](https://build.openvpn.net/downloads/releases/); kcpvpn automatically detects and matches installed TAP adapters).*

#### 🐧 Linux Client:
```bash
sudo ./kcpvpn_linux_amd64 \
  -mode client \
  -addr "YOUR_SERVER_IP:4000" \
  -psk "YourSuperSecretPassword" \
  -obfs stun \
  -pad-max 128
```

#### 🍎 macOS Client:
```bash
sudo ./kcpvpn_darwin_arm64 \
  -mode client \
  -addr "YOUR_SERVER_IP:4000" \
  -psk "YourSuperSecretPassword" \
  -obfs stun \
  -pad-max 128
```

---

## 📋 CLI Flag Reference

| Flag | Default | Applicable | Description |
| :--- | :--- | :--- | :--- |
| `-mode` | `server` | All | Operating mode: `server` or `client` |
| `-addr` | `127.0.0.1:4000` | All | Listen or destination UDP socket address |
| `-psk` | `HereIsYour...` | All | Pre-Shared Key (PSK) |
| `-obfs` | `none` | All | Obfuscation mode: `none`, `stun`, `dns`, `dtls`, `quic`, `xor` |
| `-pad-max` | `0` | All | Maximum random padding bytes per packet (`0` to disable) |
| `-crypt` | `salsa20` | All | KCP encryption cipher: `salsa20`, `aes`, `none` |
| `-mtu` | `1350` | All | KCP Maximum Transmission Unit (MTU) |
| `-sndwnd` | `4096` | All | KCP Send Window Size |
| `-rcvwnd` | `4096` | All | KCP Receive Window Size |
| `-fec-d` | `5` | All | Reed-Solomon FEC Data Shards |
| `-fec-p` | `2` | All | Reed-Solomon FEC Parity Shards |
| `-dscp` | `0` | All | IP DSCP Quality of Service tag (e.g. `46` for EF queue) |
| `-tap` | `tap0` | All | TAP interface device name |
| `-v4net` | `10.0.0.0/24` | Server | Server IPv4 CIDR virtual network |
| `-v6net` | `fd00::/64` | Server | Server IPv6 CIDR virtual network |
| `-ip4` | `""` | Client | Client static requested IPv4 address |
| `-fwmark` | `0` | Client | Policy routing fwmark tag (Linux only, e.g. `1911`) |
| `-loglevel`| `warn` | All | Log level: `debug`, `info`, `warn`, `error` |
| `-v` | `false` | All | Print version information and exit |

---

## 📦 Packet Structure & Wire Format

```text
+---------------------------------------------------------------------------------------+
| 1. Outer Transport Headers (IP & UDP)                                                 |
|    ├── IP Header (20 Bytes, contains -dscp QoS tag)                                   |
|    └── UDP Header (8 Bytes, contains Src/Dst Ports)                                   |
+---------------------------------------------------------------------------------------+
| 2. KCP ARQ & Crypto Layer (AES / Salsa20)                                             |
|    ├── KCP Header (24 Bytes: Conv, Cmd, Frg, Wnd, TS, Sn, Una, Len)                   |
|    └── Block Cipher Encryption (Encrypts all payloads below)                          |
+=======================================================================================+
| 3. Obfuscation Header (-obfs)                                                         |
|    ├── obfs=stun : STUN Header (20 Bytes: Magic Cookie 0x2112A442 & Transaction ID)   |
|    ├── obfs=dns  : DNS Header  (12 Bytes: Transaction ID, Flags, QD/AN Counts)        |
|    ├── obfs=dtls : DTLS Record (13 Bytes: ContentType 0x17, Version 0xFEFF, Seq)      |
|    ├── obfs=quic : QUIC Header (8 Bytes: Connection ID & Packet Number)               |
|    ├── obfs=xor  : No header (Applies PSK-derived XOR stream mask)                    |
|    └── obfs=none : No camouflage header (0 Bytes)                                     |
+---------------------------------------------------------------------------------------+
| 4. Inner Frame Header (3 Bytes)                                                       |
|    ├── Payload Length: 2 Bytes (Big-Endian uint16)                                    |
|    └── Padding Length (p): 1 Byte (uint8, range: 0 ~ pad-max)                         |
+---------------------------------------------------------------------------------------+
| 5. Raw Layer-2 Ethernet Frame (Length = Payload Length)                               |
|    ├── Destination MAC: 6 Bytes                                                       |
|    ├── Source MAC: 6 Bytes                                                            |
|    ├── EtherType: 2 Bytes (0x0800 IPv4 / 0x0806 ARP / 0x86DD IPv6)                    |
|    └── IP / ARP Packet Data                                                           |
+---------------------------------------------------------------------------------------+
| 6. Dynamic Random Padding Tail (Length = p Bytes)                                     |
|    └── Cryptographic Random Noise Bytes                                               |
+---------------------------------------------------------------------------------------+
```

---

## 📜 License

Distributed under the [GPL-3.0 License](LICENSE).