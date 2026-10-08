package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/redhat-appstudio/konflux-test/internal/config"
)

type Runner interface {
	Run(context.Context, config.Request) error
}

type CleanupRunner interface {
	Cleanup(context.Context, config.Request) error
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
		if req.Provider == config.ProviderBoth {
			return runBoth(ctx, req, runner)
		}
		return runner.Run(ctx, req)
	case config.CommandCleanup:
		cleanupRunner, ok := runner.(CleanupRunner)
		if !ok {
			return errors.New("cleanup services are not configured")
		}
		return cleanupRunner.Cleanup(ctx, req)
	default:
		return errors.New("unsupported command")
	}
}

func runBoth(ctx context.Context, request config.Request, runner Runner) error {
	requests := []config.Request{requestForProvider(request, config.ProviderGitHub), requestForProvider(request, config.ProviderGitLab)}
	type runResult struct {
		provider string
		err      error
	}
	results := make(chan runResult, len(requests))
	for _, providerRequest := range requests {
		go func(providerRequest config.Request) {
			results <- runResult{provider: providerRequest.Provider, err: runner.Run(ctx, providerRequest)}
		}(providerRequest)
	}
	byProvider := make(map[string]error, len(requests))
	for range requests {
		result := <-results
		byProvider[result.provider] = result.err
	}
	errorsByProvider := make([]error, 0, len(requests))
	for _, providerRequest := range requests {
		if err := byProvider[providerRequest.Provider]; err != nil {
			errorsByProvider = append(errorsByProvider, fmt.Errorf("%s run: %w", providerRequest.Provider, err))
		}
	}
	return errors.Join(errorsByProvider...)
}

func requestForProvider(request config.Request, provider string) config.Request {
	request.Provider = provider
	request.RunID = "run-" + uuid.NewString()
	request.ResumeRunID = ""
	switch provider {
	case config.ProviderGitHub:
		request.SourceRepository = config.DefaultGitHubRepo
		request.TenantNamespace = config.DefaultGitHubTenantNamespace
	case config.ProviderGitLab:
		request.SourceRepository = config.DefaultGitLabRepo
		request.TenantNamespace = config.DefaultGitLabTenantNamespace
	}
	return request
}
