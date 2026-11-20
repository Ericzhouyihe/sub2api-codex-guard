package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/codexguard/sub2api-codex-guard/internal/guard"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

func main() {
	engine := guard.New()
	defer engine.Close()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		engine.Close()
		os.Exit(0)
	}()
	pluginv1.Serve(engine)
}
