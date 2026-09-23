package main

import (
	"flag"
	"log"

	"github.com/yaojiangfeng/gpu-panel/internal/agent"
)

func main() {
	cfgPath := flag.String("config", "", "agent 配置文件路径（默认 ./agent.json、/etc/gpu-panel/agent.json）")
	mock := flag.Bool("mock", false, "使用假 GPU 数据（本机无显卡时联调用）")
	mockGPUs := flag.Int("mock-gpus", 4, "mock 模式下的显卡数量")
	flag.Parse()

	cfg, err := agent.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if *mock {
		cfg.Mock = true
		cfg.MockGPUs = *mockGPUs
	}
	agent.Run(cfg)
}
