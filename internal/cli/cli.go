package cli

import (
	"context"
	"errors"

	"github.com/redhat-appstudio/konflux-test/internal/config"
)

type Runner interface {
	Run(context.Context, config.Request) error
	CollectLogs(context.Context, config.Request) error
	Cleanup(context.Context, config.Request) error
}

type UnconfiguredRunner struct{}

func (UnconfiguredRunner) Run(context.Context, config.Request) error {
	return errors.New("workflow services are not configured")
}

func (UnconfiguredRunner) CollectLogs(context.Context, config.Request) error {
	return errors.New("collection services are not configured")
}

func (UnconfiguredRunner) Cleanup(context.Context, config.Request) error {
	return errors.New("cleanup services are not configured")
}

func Execute(ctx context.Context, req config.Request, runner Runner) error {
	if runner == nil {
		runner = UnconfiguredRunner{}
	}
	switch req.Command {
	case config.CommandRun:
		return runner.Run(ctx, req)
	case config.CommandCollectLogs:
		return runner.CollectLogs(ctx, req)
	case config.CommandCleanup:
		return runner.Cleanup(ctx, req)
	default:
		return errors.New("unsupported command")
	}
}
