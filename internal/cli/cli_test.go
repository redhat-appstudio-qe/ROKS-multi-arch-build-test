package cli

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/konflux-test/internal/config"
)

type recordingRunner struct{ command string }

func (r *recordingRunner) Run(context.Context, config.Request) error {
	r.command = config.CommandRun
	return nil
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
})
