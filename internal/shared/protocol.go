package shared

// ProtocolVersion 变更时服务端会拒绝不兼容的 agent
const ProtocolVersion = 1

// GPU 单张显卡的采样指标，缺省值 -1 表示驱动不支持该字段
type GPU struct {
	Index      int     `json:"index"`
	Name       string  `json:"name"`
	UUID       string  `json:"uuid"`
	Util       float64 `json:"util"`        // 核心利用率 %
	MemUsed    float64 `json:"mem_used"`    // MiB
	MemTotal   float64 `json:"mem_total"`   // MiB
	Temp       float64 `json:"temp"`        // 摄氏度
	Power      float64 `json:"power"`       // W
	PowerLimit float64 `json:"power_limit"` // W
	Fan        float64 `json:"fan"`         // %
	ClockSM    float64 `json:"clock_sm"`    // MHz
	ClockMem   float64 `json:"clock_mem"`   // MHz
	PCIeGen    int     `json:"pcie_gen"`
	PCIeWidth  int     `json:"pcie_width"`
}

type CPU struct {
	Usage  float64 `json:"usage"` // %
	Cores  int     `json:"cores"`
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type Mem struct {
	Total   float64 `json:"total"` // MiB
	Used    float64 `json:"used"`  // MiB
	Percent float64 `json:"percent"`
}

type Disk struct {
	Mount   string  `json:"mount"`
	Total   float64 `json:"total"` // GiB
	Used    float64 `json:"used"`  // GiB
	Percent float64 `json:"percent"`
}

type Net struct {
	RxRate float64 `json:"rx_rate"` // KiB/s
	TxRate float64 `json:"tx_rate"` // KiB/s
}

// Snapshot agent 每次上报的完整状态
type Snapshot struct {
	AgentID  string `json:"agent_id"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	TS       int64  `json:"ts"`     // unix 毫秒
	Uptime   uint64 `json:"uptime"` // 秒
	CPU      CPU    `json:"cpu"`
	Mem      Mem    `json:"mem"`
	Disks    []Disk `json:"disks"`
	Net      Net    `json:"net"`
	GPUs     []GPU  `json:"gpus"`
}

// MachineView 面板使用的机器视图（快照 + 服务端补充字段）
type MachineView struct {
	AgentID  string `json:"agent_id"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Version  string `json:"version"`
	Online   bool   `json:"online"`
	LastSeen int64  `json:"last_seen"`
	TS       int64  `json:"ts"`
	Uptime   uint64 `json:"uptime"`
	CPU      CPU    `json:"cpu"`
	Mem      Mem    `json:"mem"`
	Disks    []Disk `json:"disks"`
	Net      Net    `json:"net"`
	GPUs     []GPU  `json:"gpus"`
}

type Alert struct {
	Kind     string `json:"kind"` // offline / gpu_temp / gpu_drop / gpu_idle
	Level    string `json:"level"`
	AgentID  string `json:"agent_id"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	GPU      int    `json:"gpu"` // 与机器无关的告警为 -1
	Message  string `json:"message"`
	Since    int64  `json:"since"`
}

// PanelState 推送给浏览器面板的全量状态
type PanelState struct {
	Now      int64         `json:"now"`
	Machines []MachineView `json:"machines"`
	Alerts   []Alert       `json:"alerts"`
}
