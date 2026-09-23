# GPU Panel · 矿机 GPU 监控面板

轻量版「哪吒面板」：Go 单二进制，服务端 + Agent 双端，零运行时依赖，主要监控 GPU 状态。
Agent 主动连出，矿机不需要开放任何入站端口。

## 特性

- **单文件部署**：静态编译，服务端约 5.5MB、Agent 约 4.9MB，无 cgo、无驱动库依赖，Linux 拷贝即用
- **GPU 为主**：利用率 / 显存 / 温度 / 功耗 / 风扇 / 核心与显存时钟 / PCIe 链路
- **主机为辅**：CPU、内存、磁盘、网络速率、负载、运行时长
- **实时 + 历史**：WebSocket 秒级刷新，服务端内存环形缓冲保存趋势（默认 5s 一点 × 720 点 ≈ 1 小时）
- **告警**：矿机离线、显卡高温、掉卡（显卡数量减少）、可选长时间低利用率（疑似停挖）
- **企业微信推送**：告警触发与恢复均推送到群机器人，带冷却去重，不刷屏
- **资源占用低**：服务端常驻内存约 10–20MB，Agent 约 8–15MB

## 架构

```
矿机 Agent ──WebSocket 上报──▶ 服务端（单二进制，前端内嵌）──▶ 浏览器面板
```

- Agent 每 2s（可配）用 `nvidia-smi --query-gpu` 采集一次并上报
- 服务端内存保存最新快照，按间隔采样写入环形缓冲
- 浏览器通过 `/ws/viewer` 订阅，状态变化即时推送（节流 900ms）

## 快速开始

### 1. 编译（在开发机上）

```bash
./build.sh linux amd64        # 产物在 dist/linux-amd64/
```

### 2. 部署服务端（有公网 IP 的机器）

```bash
cp dist/linux-amd64/gpu-panel-server /usr/local/bin/
mkdir -p /etc/gpu-panel
cp config.example.json /etc/gpu-panel/config.json
# 编辑 config.json：修改 token（必改）、listen、告警阈值
cp deploy/gpu-panel-server.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now gpu-panel-server
journalctl -u gpu-panel-server -f
```

首次启动会打印 agent 接入地址，形如：
`ws://<公网IP>:8080/ws/agent?token=xxxx`

浏览器打开 `http://<公网IP>:8080` 即可。

### 3. 部署 Agent（每台矿机）

```bash
scp dist/linux-amd64/gpu-panel-agent root@<矿机IP>:/tmp/
# 在矿机上（脚本需 root）
sudo ./deploy/install-agent.sh \
  --server ws://<服务端公网IP>:8080/ws/agent \
  --token <与服务端一致的 token> \
  --id rig-01
```

或手动安装：把二进制放 `/usr/local/bin/`，配置写 `/etc/gpu-panel/agent.json`，再用 `deploy/gpu-panel-agent.service`。

### 4. 本地联调（无显卡的机器）

```bash
./build.sh darwin arm64           # 或 linux amd64
cp config.example.json /tmp/config.json   # 改 token
./dist/darwin-arm64/gpu-panel-server -config /tmp/config.json &
GPU_AGENT_SERVER=ws://127.0.0.1:8080/ws/agent GPU_AGENT_TOKEN=<token> \
  ./dist/darwin-arm64/gpu-panel-agent -mock -mock-gpus 4
```

`-mock` 会生成 4 张 RTX 5070 Ti 的仿真数据，用于验证面板。

## 配置

### 服务端 `config.json`

| 字段 | 默认 | 说明 |
|---|---|---|
| `listen` | `:8080` | 监听地址 |
| `token` | 随机生成 | Agent 接入令牌，必改 |
| `panel_user` / `panel_pass` | 空 | 面板 Basic 认证，留空则不启用 |
| `alerts.offline_after_sec` | 30 | 超过该时间未上报判定离线 |
| `alerts.gpu_temp_threshold` | 80 | GPU 温度告警阈值（°C），超阈值+7 升级为严重 |
| `alerts.gpu_idle_threshold` | 0 | 利用率低于该值告警，0 关闭 |
| `alerts.gpu_idle_after_sec` | 600 | 低利用率持续时间阈值 |
| `history.interval_sec` | 5 | 历史采样间隔 |
| `history.retention_points` | 720 | 每个序列保留点数 |
| `notify.enabled` | true | 企业微信推送总开关 |
| `notify.wecom_webhook` | 空 | 群机器人 Webhook 地址，留空则不推送 |
| `notify.levels` | crit / warn | 推送哪些级别的告警 |
| `notify.cooldown_sec` | 600 | 同一告警的推送冷却时间 |
| `notify.recovery` | true | 告警恢复时是否推送 |
| `notify.timeout_sec` | 10 | 单次推送超时 |

环境变量可覆盖：`GPU_PANEL_TOKEN`、`GPU_PANEL_LISTEN`、`GPU_PANEL_USER`、`GPU_PANEL_PASS`、`GPU_PANEL_WECOM_WEBHOOK`。

### 企业微信推送

1. 在企业微信群 → 群设置 → 群机器人 → 添加机器人，复制 Webhook 地址
2. 填进 `config.json` 的 `notify.wecom_webhook`
3. 自检（会往群里发一条测试消息，然后退出）：

```bash
gpu-panel-server -config /etc/gpu-panel/config.json -test-notify
```

推送行为：

- 告警**首次出现**时推送一次，持续期间不重复（按 `告警类型|机器|显卡` 去重）
- 同一告警在 `cooldown_sec` 内反复出现只推一次，避免抖动刷屏
- 告警**消失**时推送恢复消息（矿机离线导致的 GPU 告警消失不算恢复，不推送）
- 群机器人限流 20 条/分钟，推送串行间隔 1 秒；发送失败只记日志，不影响监控

### Agent `agent.json`

| 字段 | 默认 | 说明 |
|---|---|---|
| `server` | — | `ws://<服务端IP>:8080/ws/agent` |
| `token` | — | 与服务端一致 |
| `agent_id` | 主机名 | 面板显示的机器标识 |
| `interval_ms` | 2000 | 上报间隔 |
| `nvidia_smi` | 空 | nvidia-smi 路径，留空用 PATH |
| `mock` / `mock_gpus` | false / 4 | 联调用的假数据开关 |

环境变量可覆盖：`GPU_AGENT_SERVER`、`GPU_AGENT_TOKEN`、`GPU_AGENT_ID`。

## 面板说明

- 顶部汇总：在线矿机数、GPU 总数、平均利用率、总功耗、严重告警数
- 告警条：离线（红）、高温（≥阈值黄、≥阈值+7 红）、掉卡、长时间低利用率
- 每台矿机一张卡片：CPU / 内存 / 负载 / 网络 / 磁盘 + 每张 GPU 的状态条与 90 点趋势曲线（蓝=利用率，橙=温度）

## 已知限制

- 历史数据只在内存中，服务端重启后曲线清空（默认保留 1 小时，避免内存膨胀）；需要长期留存可加 SQLite
- GPU 采集依赖 `nvidia-smi`（无 NVML/cgo 依赖，因此不需要编译期 CUDA 头文件）
- 告警只在面板内展示，未接外部推送渠道
- 掉卡判定基于「本次上报显卡数 < 历史最大数」，矿机热插拔或驱动异常重启后需重启 Agent 复位
