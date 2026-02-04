package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danni2019/starSling/internal/logging"
	"github.com/danni2019/starSling/internal/router"
	"github.com/danni2019/starSling/internal/tui"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := logging.New("INFO")
	routerState := router.NewState()
	routerServer, err := router.Start(ctx, "127.0.0.1:0", routerState, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start router:", err)
		return 1
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		_ = routerServer.Stop(stopCtx)
	}()

	return tui.Run(ctx, routerServer.Addr(), logger)
}
