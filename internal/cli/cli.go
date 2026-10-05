package cli

import (
	"context"
	"errors"

	"github.com/redhat-appstudio/konflux-test/internal/config"
)

type Runner interface {
	Run(context.Context, config.Request) error
}

type UnconfiguredRunner struct{}

func (UnconfiguredRunner) Run(context.Context, config.Request) error {
	return errors.New("workflow services are not configured")
}

func Execute(ctx context.Context, req config.Request, runner Runner) error {
	if runner == nil {
		runner = UnconfiguredRunner{}
	}
	switch req.Command {
	case config.CommandRun:
		return runner.Run(ctx, req)
	default:
		return errors.New("unsupported command")
	}
}
