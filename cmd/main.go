package main

import (
	"e-commerce/internal/app"
	"log"
)

func main() {
	ctx, stop, conf, err := app.Bootstrap()
	if err != nil {
		log.Fatalf("应用启动失败：%v", err)
	}

	defer stop()

	app.Run(ctx, *conf)
}
