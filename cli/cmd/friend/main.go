package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/activityforge/activityforge/internal/cli"
	"github.com/activityforge/activityforge/internal/quickstart"
)

func main() {
	if len(os.Args) > 1 {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		app := cli.App{}
		if err := app.Run(ctx, os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	launcher := quickstart.Launcher{}
	if err := launcher.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
