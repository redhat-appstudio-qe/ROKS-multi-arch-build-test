package cluster

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeCluster struct {
	server      string
	checks      []CheckResult
	scheduling  []CheckResult
	accessCalls int
}

func (f *fakeCluster) Server(context.Context) (string, error) { return f.server, nil }
func (f *fakeCluster) CheckAccess(context.Context, []AccessCheck) []CheckResult {
	f.accessCalls++
	return f.checks
}
func (f *fakeCluster) CheckScheduling(context.Context, SchedulingSpec) []CheckResult {
	return f.scheduling
}

var _ = Describe("RunPreflight", func() {
	It("rejects wrong server without access check", func() {
		fake := &fakeCluster{server: "https://actual.example"}
		_, err := RunPreflight(context.Background(), fake, PreflightSpec{ExpectedServer: "https://expected.example"})
		Expect(err).To(HaveOccurred())
		Expect(fake.accessCalls).To(Equal(0))
	})

	It("rejects failed controller scheduling", func() {
		fake := &fakeCluster{server: "https://api.example", scheduling: []CheckResult{{Name: "multi-platform-controller/linux/arm64", Details: "platform is not configured in host-config"}}}
		_, err := RunPreflight(context.Background(), fake, PreflightSpec{ExpectedServer: "https://api.example", Scheduling: SchedulingSpec{Namespace: "multi-platform-controller", Platforms: []string{"linux/arm64"}}})
		Expect(err).To(HaveOccurred())
	})

	It("requires controller scheduling readiness", func() {
		fake := &fakeCluster{
			server:     "https://api.example",
			checks:     []CheckResult{{Name: "pipelines", Passed: true}},
			scheduling: []CheckResult{{Name: "multi-platform-controller/linux/arm64", Passed: true}},
		}
		result, err := RunPreflight(context.Background(), fake, PreflightSpec{
			ExpectedServer: "https://api.example",
			Scheduling:     SchedulingSpec{Namespace: "multi-platform-controller", Platforms: []string{"linux/arm64"}},
		})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(result.Scheduling).To(HaveLen(1))
		Expect(result.Scheduling[0].Name).To(Equal("multi-platform-controller/linux/arm64"))
	})
})
