package agent

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

type hostTracker struct {
	mu       sync.Mutex
	prevIdle float64
	prevTot  float64
	prevRx   float64
	prevTx   float64
	prevAt   time.Time
}

var tracker = &hostTracker{}

// CollectHost 采集 CPU / 内存 / 磁盘 / 网络 / 负载 / 运行时长
func CollectHost() shared.Snapshot {
	snap := shared.Snapshot{CPU: shared.CPU{Cores: runtime.NumCPU()}}
	if runtime.GOOS != "linux" {
		return snap
	}
	snap.CPU.Usage = cpuUsage()
	snap.Mem = memInfo()
	snap.Disks = diskInfo()
	rx, tx := netRates()
	snap.Net = shared.Net{RxRate: rx, TxRate: tx}
	snap.Uptime = uptime()
	snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15 = loadAvg()
	return snap
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := []string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

func fields(s string) []string {
	return strings.Fields(s)
}

func cpuUsage() float64 {
	lines := readLines("/proc/stat")
	if len(lines) == 0 {
		return 0
	}
	f := fields(lines[0])
	if len(f) < 5 || f[0] != "cpu" {
		return 0
	}
	var idle, total float64
	for i := 1; i < len(f); i++ {
		v, _ := strconv.ParseFloat(f[i], 64)
		total += v
		if i == 4 || i == 5 { // idle, iowait
			idle += v
		}
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.prevTot == 0 {
		tracker.prevIdle, tracker.prevTot = idle, total
		return 0
	}
	dIdle := idle - tracker.prevIdle
	dTot := total - tracker.prevTot
	tracker.prevIdle, tracker.prevTot = idle, total
	if dTot <= 0 {
		return 0
	}
	usage := (1 - dIdle/dTot) * 100
	if usage < 0 {
		usage = 0
	}
	return usage
}

func memInfo() shared.Mem {
	total, avail := 0.0, 0.0
	for _, l := range readLines("/proc/meminfo") {
		f := fields(l)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "MemTotal:":
			total = v / 1024
		case "MemAvailable:":
			avail = v / 1024
		}
	}
	used := total - avail
	if used < 0 {
		used = 0
	}
	pct := 0.0
	if total > 0 {
		pct = used / total * 100
	}
	return shared.Mem{Total: total, Used: used, Percent: pct}
}

var diskFSTypes = map[string]bool{
	"ext4": true, "ext3": true, "ext2": true, "xfs": true,
	"btrfs": true, "f2fs": true, "zfs": true, "overlay": true,
}

func diskInfo() []shared.Disk {
	out := []shared.Disk{}
	seen := map[string]bool{}
	for _, l := range readLines("/proc/mounts") {
		f := fields(l)
		if len(f) < 3 || !diskFSTypes[f[2]] || seen[f[1]] {
			continue
		}
		if strings.HasPrefix(f[1], "/boot") || strings.HasPrefix(f[1], "/proc") {
			continue
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(f[1], &st); err != nil {
			continue
		}
		total := float64(st.Blocks) * float64(st.Bsize) / 1073741824
		free := float64(st.Bavail) * float64(st.Bsize) / 1073741824
		used := total - free
		if total <= 0 {
			continue
		}
		seen[f[1]] = true
		out = append(out, shared.Disk{
			Mount:   f[1],
			Total:   total,
			Used:    used,
			Percent: used / total * 100,
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func netRates() (float64, float64) {
	rx, tx := 0.0, 0.0
	for _, l := range readLines("/proc/net/dev") {
		if !strings.Contains(l, ":") {
			continue
		}
		parts := strings.SplitN(l, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" || strings.HasPrefix(iface, "docker") || strings.HasPrefix(iface, "veth") {
			continue
		}
		f := fields(parts[1])
		if len(f) < 10 {
			continue
		}
		v1, _ := strconv.ParseFloat(f[0], 64)
		v2, _ := strconv.ParseFloat(f[8], 64)
		rx += v1
		tx += v2
	}
	now := time.Now()
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.prevAt.IsZero() {
		tracker.prevRx, tracker.prevTx, tracker.prevAt = rx, tx, now
		return 0, 0
	}
	elapsed := now.Sub(tracker.prevAt).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	dr := (rx - tracker.prevRx) / elapsed / 1024
	dt := (tx - tracker.prevTx) / elapsed / 1024
	tracker.prevRx, tracker.prevTx, tracker.prevAt = rx, tx, now
	if dr < 0 {
		dr = 0
	}
	if dt < 0 {
		dt = 0
	}
	return dr, dt
}

func uptime() uint64 {
	lines := readLines("/proc/uptime")
	if len(lines) == 0 {
		return 0
	}
	f := fields(lines[0])
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return uint64(v)
}

func loadAvg() (float64, float64, float64) {
	lines := readLines("/proc/loadavg")
	if len(lines) == 0 {
		return 0, 0, 0
	}
	f := fields(lines[0])
	if len(f) < 3 {
		return 0, 0, 0
	}
	a, _ := strconv.ParseFloat(f[0], 64)
	b, _ := strconv.ParseFloat(f[1], 64)
	c, _ := strconv.ParseFloat(f[2], 64)
	return a, b, c
}
