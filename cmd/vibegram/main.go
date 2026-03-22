package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signalContext(os.Args[1:])
	defer stop()

	if err := runArgs(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	return runArgs(ctx, nil, os.Stdin, os.Stdout, os.Stderr)
}

func signalContext(args []string) (context.Context, context.CancelFunc) {
	if shouldUseSignalContext(args) {
		return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	}

	return context.WithCancel(context.Background())
}

func shouldUseSignalContext(args []string) bool {
	if len(args) == 0 {
		return true
	}

	return args[0] == "daemon"
}
