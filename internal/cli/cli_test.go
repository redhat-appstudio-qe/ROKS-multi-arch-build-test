package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/konflux-test/internal/config"
)

type recordingRunner struct{ command string }

func (r *recordingRunner) Run(context.Context, config.Request) error {
	r.command = config.CommandRun
	return nil
}

type concurrentRecordingRunner struct {
	mu       sync.Mutex
	requests []config.Request
	started  chan string
	release  <-chan struct{}
	failures map[string]error
}

type cleanupRecordingRunner struct {
	cleanupCalls int
	request      config.Request
}

func (*cleanupRecordingRunner) Run(context.Context, config.Request) error { return nil }

func (r *cleanupRecordingRunner) Cleanup(_ context.Context, request config.Request) error {
	r.cleanupCalls++
	r.request = request
	return nil
}

func (r *concurrentRecordingRunner) Run(ctx context.Context, request config.Request) error {
	r.mu.Lock()
	r.requests = append(r.requests, request)
	r.mu.Unlock()
	if r.started != nil {
		r.started <- request.Provider
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.failures[request.Provider]
}

func (r *concurrentRecordingRunner) recorded() []config.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]config.Request(nil), r.requests...)
}

var _ = Describe("Execute", func() {
	It("dispatches run", func() {
		runner := &recordingRunner{}
		req := config.Defaults()
		req.Command = config.CommandRun
		req.ClusterServer = "https://api.example"
		req.Provider = config.ProviderGitHub
		if err := Execute(context.Background(), req, runner); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(runner.command).To(Equal(config.CommandRun))
	})

	It("runs both fixed providers concurrently with separate namespaces and run IDs", func() {
		GinkgoT().Setenv("GITHUB_TOKEN", "github-token")
		GinkgoT().Setenv("GITLAB_BOT_TOKEN", "gitlab-token")
		request, err := config.Parse([]string{"run", "both", "--cluster-server", "https://api.example"})
		Expect(err).ShouldNot(HaveOccurred())

		release := make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		runner := &concurrentRecordingRunner{started: make(chan string, 2), release: release}
		done := make(chan error, 1)
		go func() { done <- Execute(context.Background(), request, runner) }()

		started := map[string]bool{}
		for range 2 {
			select {
			case provider := <-runner.started:
				started[provider] = true
			case <-time.After(time.Second):
				Fail("both provider workflows should start before either is released")
			}
		}
		close(release)
		released = true
		Expect(<-done).Should(Succeed())
		Expect(started).Should(HaveKey(config.ProviderGitHub))
		Expect(started).Should(HaveKey(config.ProviderGitLab))

		requests := runner.recorded()
		Expect(requests).Should(HaveLen(2))
		byProvider := map[string]config.Request{}
		for _, providerRequest := range requests {
			byProvider[providerRequest.Provider] = providerRequest
		}
		Expect(byProvider[config.ProviderGitHub].TenantNamespace).Should(Equal(config.DefaultGitHubTenantNamespace))
		Expect(byProvider[config.ProviderGitLab].TenantNamespace).Should(Equal(config.DefaultGitLabTenantNamespace))
		Expect(byProvider[config.ProviderGitHub].SourceRepository).Should(Equal(config.DefaultGitHubRepo))
		Expect(byProvider[config.ProviderGitLab].SourceRepository).Should(Equal(config.DefaultGitLabRepo))
		Expect(byProvider[config.ProviderGitHub].StateDir).Should(Equal(byProvider[config.ProviderGitLab].StateDir))
		Expect(byProvider[config.ProviderGitHub].RunID).ShouldNot(BeEmpty())
		Expect(byProvider[config.ProviderGitLab].RunID).ShouldNot(Equal(byProvider[config.ProviderGitHub].RunID))
	})

	It("aggregates failures from both provider runs", func() {
		GinkgoT().Setenv("GITHUB_TOKEN", "github-token")
		GinkgoT().Setenv("GITLAB_BOT_TOKEN", "gitlab-token")
		request, err := config.Parse([]string{"run", "both", "--cluster-server", "https://api.example"})
		Expect(err).ShouldNot(HaveOccurred())
		runner := &concurrentRecordingRunner{failures: map[string]error{
			config.ProviderGitHub: errors.New("github failed"),
			config.ProviderGitLab: errors.New("gitlab failed"),
		}}

		err = Execute(context.Background(), request, runner)
		Expect(err).Should(MatchError(And(ContainSubstring("github failed"), ContainSubstring("gitlab failed"))))
	})

	It("dispatches cleanup using the requested run ID", func() {
		request, err := config.Parse([]string{"cleanup", "--run-id", "run-123", "--cluster-server", "https://api.example"})
		Expect(err).ShouldNot(HaveOccurred())
		runner := &cleanupRecordingRunner{}

		err = Execute(context.Background(), request, runner)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(runner.cleanupCalls).Should(Equal(1))
		Expect(runner.request.RunID).Should(Equal("run-123"))
	})
})
