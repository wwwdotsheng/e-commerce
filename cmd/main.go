package main

import (
	"e-commerce/internal/app"
	"flag"
	"log"
)

func main() {
	configPath := flag.String("c", "configs/config.yaml", "path to config file")
	flag.Parse()

	ctx, stop, conf, err := app.Bootstrap(*configPath)
	if err != nil {
		log.Fatalf("应用启动失败：%v", err)
	}

	defer stop()

	app.Run(ctx, *conf)
}
