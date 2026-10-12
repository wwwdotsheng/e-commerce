package main

import (
	"e-commerce/internal/app"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configPath := flag.String("c", "", "configuration YAML path (overrides CONFIG_PATH)")
	dotenvPath := flag.String("dotenv", "", "optional dotenv path (overrides DOTENV_PATH)")
	flag.Parse()
	if *configPath != "" {
		if err := os.Setenv("CONFIG_PATH", *configPath); err != nil {
			log.Fatal(err)
		}
	}
	if *dotenvPath != "" {
		if err := os.Setenv("DOTENV_PATH", *dotenvPath); err != nil {
			log.Fatal(err)
		}
	}
	ctx, stop, conf, err := app.Bootstrap()
	if err != nil {
		log.Fatalf("应用启动失败：%v", err)
	}
	defer stop()

	// 捕获系统信号，实现优雅关闭
	sigCtx, sigCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer sigCancel()

	// 在 goroutine 中启动服务
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx, *conf)
	}()

	// 等待信号或启动错误
	select {
	case <-sigCtx.Done():
		fmt.Println("\n收到关闭信号，正在优雅退出...")
	case err := <-errCh:
		if err != nil {
			log.Printf("服务异常退出: %v\n", err)
		}
		sigCancel()
	}

	// stop() 在 defer 中执行：关闭 MQ → DB → OTel → cancel
	fmt.Println("服务已关闭")
}
