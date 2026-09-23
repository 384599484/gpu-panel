package agent

import (
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

var gpuQuery = []string{
	"index", "name", "uuid", "utilization.gpu", "memory.used", "memory.total",
	"temperature.gpu", "power.draw", "power.limit", "fan.speed",
	"clocks.sm", "clocks.memory", "pcie.link.gen.current", "pcie.link.width.current",
}

var nvidiaSmiPath = "nvidia-smi"

// CollectGPUs 通过 nvidia-smi 采集显卡指标，驱动不支持的字段返回 -1
func CollectGPUs() ([]shared.GPU, error) {
	out, err := exec.Command(nvidiaSmiPath,
		"--query-gpu="+strings.Join(gpuQuery, ","),
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, err
	}
	return parseGPUCSV(string(out))
}

func parseGPUCSV(s string) ([]shared.GPU, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(s)))
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	gpus := make([]shared.GPU, 0, len(rows))
	for _, row := range rows {
		if len(row) < len(gpuQuery) {
			continue
		}
		g := shared.GPU{
			Name:       strings.TrimSpace(row[1]),
			UUID:       strings.TrimSpace(row[2]),
			Util:       num(row[3]),
			MemUsed:    num(row[4]),
			MemTotal:   num(row[5]),
			Temp:       num(row[6]),
			Power:      num(row[7]),
			PowerLimit: num(row[8]),
			Fan:        num(row[9]),
			ClockSM:    num(row[10]),
			ClockMem:   num(row[11]),
			PCIeGen:    int(num(row[12])),
			PCIeWidth:  int(num(row[13])),
		}
		idx, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil {
			idx = len(gpus)
		}
		g.Index = idx
		gpus = append(gpus, g)
	}
	return gpus, nil
}

// num 解析 nvidia-smi 输出，无法识别（N/A、[Not Supported] 等）返回 -1
func num(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v
}

var (
	gpuWarnOnce sync.Once
)

func gpuUnavailable(err error) {
	gpuWarnOnce.Do(func() {
		logf("采集 GPU 失败（%v），将只上报主机指标", err)
	})
}
