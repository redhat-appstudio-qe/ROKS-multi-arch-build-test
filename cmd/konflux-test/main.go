package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/redhat-appstudio/konflux-test/internal/cli"
	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/setup"
)

// main runs konflux-test and exits with a nonzero status when execution fails.
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run prepares, parses, and executes the supplied command-line arguments.
func run(args []string) error {
	preparedArgs, err := setup.Prepare(args)
	if err != nil {
		return err
	}
	req, err := config.Parse(preparedArgs)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cli.Execute(ctx, req, cli.NewLiveRunner())
}
