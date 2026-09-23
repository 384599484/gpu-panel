package main

import (
	"flag"
	"log"

	"github.com/yaojiangfeng/gpu-panel/internal/server"
)

func main() {
	cfgPath := flag.String("config", "", "配置文件路径（默认依次尝试 ./config.json、/etc/gpu-panel/config.json）")
	saveCfg := flag.Bool("save-config", false, "把当前生效配置回写到配置文件")
	testNotify := flag.Bool("test-notify", false, "向企业微信发送一条自检消息后退出")
	flag.Parse()

	cfg, err := server.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if *testNotify {
		if err := server.New(cfg).NotifyTest(); err != nil {
			log.Fatalf("推送自检失败: %v", err)
		}
		log.Printf("自检消息已发送，请到群里查看")
		return
	}
	if *saveCfg {
		if err := cfg.Save(); err != nil {
			log.Fatalf("写入配置失败: %v", err)
		}
		log.Printf("配置已写入 %s", cfg.ConfigPath)
		return
	}

	if err := server.New(cfg).Run(); err != nil {
		log.Fatalf("服务端退出: %v", err)
	}
}
