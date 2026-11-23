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
- 🎭 **Anti-DPI Multi-Protocol Camouflage (`obfs` config)**:
  - `stun`: **WebRTC Audio/Video Call Camouflage (RFC 5389)**, disguising traffic as Discord, Google Meet, or Zoom calls.
  - `dns`: **DNS Query Camouflage (RFC 1035)**, wrapping packets as standard UDP DNS 53 queries and responses.
  - `dtls`: **DTLS 1.2 Encrypted Record Camouflage**, disguising traffic as Cisco AnyConnect / OpenVPN SSL VPN.
  - `quic`: **HTTP/3 QUIC Header Camouflage**, disguising traffic as Cloudflare / Google HTTP/3 web traffic.
  - `xor`: **Dynamic Byte Masking**, obscuring packet boundaries with PSK-derived key masks.
- 🎲 **Dynamic Random Packet Padding (`pad-max` config)**:
  - Appends `0` to `N` random padding bytes at the end of each packet to distort length statistics and defeat DPI fingerprinting.
- 🔀 **Layer-2 Ethernet Switching & Dynamic IPAM**:
  - Built-in MAC address learning table with unicast and multicast packet forwarding.
  - Parses IPv4 & IPv6 CIDR subnets, dynamically distributing client IP, gateway, mask, and supporting static IP reservations.
- 🛡️ **Forward Error Correction (FEC) & Authenticated Encryption**:
  - Reed-Solomon FEC (configurable data & parity shards ratio).
  - Default **AES-GCM authenticated encryption** (`-crypt aes-gcm`), plus AES-CFB and Salsa20; the key is derived from the PSK via **PBKDF2** (40,960 iterations) to raise brute-force cost.
- 🎛️ **QoS & Transport Tuning**:
  - Supports `-dscp` QoS IP header tagging (e.g. `DSCP 46` for Expedited Low-Latency Queue).
  - `mtu` is the **tunnel interface MTU** (default **1500**); the KCP outer datagram size is auto-derived to **avoid WAN IP fragmentation**; `sndwnd` / `rcvwnd` window controls.
- 🔒 **Connection Security**:
  - `psk` Pre-Shared Key is **mandatory** (no public default anymore).
  - Server-side `max-clients` concurrent connection limit (excess rejected).
  - Layer-2 switching enforces **source MAC == registered MAC** for client frames, blocking MAC-table poisoning and source-address spoofing.
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
> **Note**: All configuration is provided via `config.json`. **The CLI only keeps `-version` and `-config`.**
> `psk` is **required**; Server and Client must use matching keys, `obfs` modes, and `pad-max` padding. Root or Administrator privileges are required to configure TAP network interfaces.

### 0. Prepare the config file

```bash
cp config.example.json config.json
# Edit config.json; at least change "psk" to a strong secret. Adjust mode/addr/v4net/obfs as needed.
```

### 1. Server Mode

```bash
# Start Server using config.json (STUN WebRTC camouflage + 128-byte random padding + DSCP 46 priority)
sudo ./kcpvpn_linux_amd64 -config config.json
```

### 2. Client Mode

#### 🪟 Windows Client:
Open PowerShell or CMD as Administrator:
```cmd
kcpvpn_windows_amd64.exe -config config.json
```
*(Requires standard [OpenVPN TAP-Windows Driver](https://build.openvpn.net/downloads/releases/); kcpvpn automatically detects and matches installed TAP adapters).*

#### 🐧 Linux Client:
```bash
sudo ./kcpvpn_linux_amd64 -config config.json
```

#### 🍎 macOS Client:
```bash
sudo ./kcpvpn_darwin_arm64 -config config.json
```

---

## 📋 config.json Configuration Reference

> Missing fields fall back to defaults. `-config` sets the config path (default `config.json`); `-version` prints the version and exits.

| Field | Default | Applicable | Description |
| :--- | :--- | :--- | :--- |
| `mode` | `server` | All | Operating mode: `server` or `client` |
| `addr` | `127.0.0.1:4000` | All | Listen or destination UDP socket address |
| `tap` | `tap0` | All | TAP interface device name |
| `v4net` | `10.0.0.0/24` | Server | Server IPv4 CIDR virtual network |
| `v6net` | `fd00::/64` | Server | Server IPv6 CIDR virtual network |
| `fec-d` | `5` | All | Reed-Solomon FEC Data Shards (`0` to disable) |
| `fec-p` | `2` | All | Reed-Solomon FEC Parity Shards (`0` to disable) |
| `crypt` | `aes-gcm` | All | KCP encryption cipher: `aes-gcm`, `salsa20`, `aes`, `none` |
| `psk` | *(required)* | All | Pre-Shared Key (PSK); used to derive the encryption key via PBKDF2 |
| `obfs` | `none` | All | Obfuscation mode: `none`, `stun`, `dns`, `dtls`, `quic`, `xor` |
| `pad-max` | `0` | All | Maximum random padding bytes per outer datagram (`0` to disable) |
| `mtu` | `1500` | All | **Tunnel interface MTU**; KCP outer datagram size is auto-derived to avoid WAN fragmentation |
| `sndwnd` | `4096` | All | KCP Send Window Size |
| `rcvwnd` | `4096` | All | KCP Receive Window Size |
| `dscp` | `0` | All | IP DSCP Quality of Service tag (e.g. `46` for EF queue) |
| `log-level` | `warn` | All | Log level: `debug`, `info`, `warn`, `error` |
| `max-clients` | `256` | Server | Maximum concurrent client connections |
| `max-rate` | `0` | Server | Per-connection client→server injection rate cap (bytes/sec, `0`=unlimited) |
| `fwmark` | `0` | Client | Policy routing fwmark tag (Linux only, e.g. `1911`) |
| `ip4` | `""` | Client | Client static requested IPv4 address |
| `ip6` | `""` | Client | Client static requested IPv6 address |
| `mac` | `""` | Client | Client requested MAC (optional; empty = server generates & delivers) |
| `web-enabled` | `false` | All | Start the built-in Web management UI |
| `web-bind` | `local` | All | Web UI bind mode: `tunnel` (tunnel IP only), `local` (127.0.0.1/::1), `public` (all public IPs) |
| `web-addr` | `:8080` | All | Web UI listen address (port source, e.g. `:8080` or `127.0.0.1:8080`) |
| `web-token` | `""` | All | Web UI access token (Bearer). Required when `web-bind=public` |

> **CLI flags**: `-version` prints the version; `-config <path>` sets the config path (default `config.json`). All other flags have been removed.

---

## 📦 Packet Structure & Wire Format

> The obfuscation layer sits **outside KCP** (it wraps each UDP datagram), so the camouflage header is really visible on the wire and can be fingerprinted as STUN / DNS / DTLS / QUIC by DPI.

```text
+---------------------------------------------------------------------------------------+
| 1. Outer Transport Headers (IP & UDP)                                                 |
|    ├── IP Header (20 Bytes, contains -dscp QoS tag)                                   |
|    └── UDP Header (8 Bytes, contains Src/Dst Ports)                                   |
+---------------------------------------------------------------------------------------+
| 2. Obfuscation Layer (-obfs) -- wraps every KCP datagram                              |
|    ├── obfs=stun : STUN Header (20 Bytes: Magic Cookie 0x2112A442 & Transaction ID)   |
|    ├── obfs=dns  : DNS Header  (12 Bytes: Transaction ID, Flags, QD/AN Counts)        |
|    ├── obfs=dtls : DTLS Record (13 Bytes: ContentType 0x17, Version 0xFEFF, Seq)      |
|    ├── obfs=quic : QUIC Header (8 Bytes: Connection ID & Packet Number)               |
|    ├── obfs=xor  : PSK-derived XOR mask over the whole datagram                       |
|    ├── obfs=none : No camouflage header (0 Bytes)                                     |
|    └── Dynamic Random Padding Tail (p Bytes) -- controlled by -pad-max                |
+---------------------------------------------------------------------------------------+
| 3. KCP ARQ & Authenticated-Crypto Layer (AES-GCM / AES / Salsa20)                     |
|    ├── Random Nonce + Auth Tag (AES-GCM)                                             |
|    ├── KCP Header (24 Bytes: Conv, Cmd, Frg, Wnd, TS, Sn, Una, Len)                   |
|    └── Encryption (encrypts everything below; AES-GCM also authenticates)             |
+=======================================================================================+
| 4. Inner Frame Header (3 Bytes)                                                       |
|    ├── Frame Type: 1 Byte (0x01=handshake, 0x02=keepalive, 0x03=ethernet data)        |
|    └── Payload Length: 2 Bytes (Big-Endian uint16)                                    |
+---------------------------------------------------------------------------------------+
| 5. Raw Layer-2 Ethernet Frame (Length = Payload Length)                               |
|    ├── Destination MAC: 6 Bytes                                                       |
|    ├── Source MAC: 6 Bytes                                                            |
|    ├── EtherType: 2 Bytes (0x0800 IPv4 / 0x0806 ARP / 0x86DD IPv6)                    |
|    └── IP / ARP Packet Data                                                           |
+---------------------------------------------------------------------------------------+
```

> **MTU note**: `-mtu 1500` sets the tunnel interface MTU to 1500. The KCP outer datagram size is auto-derived so the outer IP packet stays ≤ 1500 bytes, avoiding WAN fragmentation. Because an Ethernet frame (1514 bytes) plus protocol overhead cannot fit in a single KCP segment on a 1500-byte physical link, full-MTU frames are split into 2 KCP segments (mathematically unavoidable; this is the architectural optimum).

> **Security note**: the MAC is generated by the server inside the encrypted handshake and delivered to the client, which configures the interface so the real MAC matches the registration; the server enforces "client frame source MAC == registered MAC". Clients no longer self-report a MAC, preventing impersonation. The handshake carries a random Nonce + timestamp; the server validates a ±300s window and dedupes the Nonce to prevent replay. After handshake the data plane accepts only typed frames (0x03 data / 0x02 keepalive); violations are dropped. The data plane is rate-limited per connection (`max-rate`), and VSwitch queue-full drops are counted and logged at a throttled rate.

---

## 🌐 Web Management UI

Set `web-enabled: true` to start the built-in HTTP management UI (available on both server and client). Disabled by default.

**Bind mode `web-bind`** controls which IPs are listened on:

| Mode | Listen addresses | Notes |
| :--- | :--- | :--- |
| `local` | `127.0.0.1` + `::1` | Loopback only (default, safe) |
| `tunnel` | tunnel interface IP | Server binds `ServerV4`/`ServerV6`; client binds its assigned tunnel IP |
| `public` | `0.0.0.0` + `::` | All addresses (including public) |

**Auth**: requests carry `Authorization: Bearer <web-token>`. When `web-bind=public`, `web-token` is **required** (startup fails otherwise); for `local`/`tunnel` an empty token is allowed (local convenience).

**Features** (server): view online clients (IP/MAC/duration), kick a client, view/edit config; **client**: view tunnel state (IP/MAC/gateway/reconnects), view/edit config. Saving writes `config.json` then **auto-restarts** the process to apply (cross-platform: spawn self + exit old process; port/TAP release is mitigated by delays on both sides). `mode` cannot be switched online (edit manually + restart).

**Port source**: `web-addr` provides the port (e.g. `:8080`); the actual listen IPs come from `web-bind`.

> **Security note**: `web-bind=public` exposes the UI to the internet — set a strong `web-token`. Default `local` is localhost-only.

---

## 📜 License

Distributed under the [GPL-3.0 License](LICENSE).