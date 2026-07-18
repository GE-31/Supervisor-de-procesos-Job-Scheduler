package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	fmt.Println("proceso continuo iniciado")
	for {
		select {
		case <-ticker.C:
			fmt.Println("heartbeat")
		case sig := <-signals:
			fmt.Printf("señal %s recibida\n", sig)
			return
		}
	}
}
