package cluster

import (
	"context"
	"testing"
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
func TestRunPreflightRejectsWrongServerWithoutAccessCheck(t *testing.T) {
	fake := &fakeCluster{server: "https://actual.example"}
	_, err := RunPreflight(context.Background(), fake, PreflightSpec{ExpectedServer: "https://expected.example"})
	if err == nil || fake.accessCalls != 0 {
		t.Fatalf("err=%v accessCalls=%d", err, fake.accessCalls)
	}
}

func TestRunPreflightRejectsFailedControllerScheduling(t *testing.T) {
	fake := &fakeCluster{server: "https://api.example", scheduling: []CheckResult{{Name: "multi-platform-controller/linux/arm64", Details: "platform is not configured in host-config"}}}
	_, err := RunPreflight(context.Background(), fake, PreflightSpec{ExpectedServer: "https://api.example", Scheduling: SchedulingSpec{Namespace: "multi-platform-controller", Platforms: []string{"linux/arm64"}}})
	if err == nil {
		t.Fatal("expected missing controller scheduling error")
	}
}

func TestRunPreflightRequiresControllerSchedulingReadiness(t *testing.T) {
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
		t.Fatal(err)
	}
	if len(result.Scheduling) != 1 || result.Scheduling[0].Name != "multi-platform-controller/linux/arm64" {
		t.Fatalf("scheduling checks = %#v", result.Scheduling)
	}
}
