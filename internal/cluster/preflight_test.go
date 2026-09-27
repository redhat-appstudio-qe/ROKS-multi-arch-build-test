package cluster

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeCluster struct {
	server      string
	checks      []CheckResult
	archive     ArchiveEndpoint
	accessCalls int
}

func (f *fakeCluster) Server(context.Context) (string, error) { return f.server, nil }
func (f *fakeCluster) CheckAccess(context.Context, []AccessCheck) []CheckResult {
	f.accessCalls++
	return f.checks
}
func (f *fakeCluster) DiscoverArchive(context.Context, schema.GroupVersionResource) (ArchiveEndpoint, error) {
	return f.archive, nil
}

func TestRunPreflightRejectsWrongServerWithoutAccessCheck(t *testing.T) {
	fake := &fakeCluster{server: "https://actual.example"}
	_, err := RunPreflight(context.Background(), fake, PreflightSpec{ExpectedServer: "https://expected.example"})
	if err == nil || fake.accessCalls != 0 {
		t.Fatalf("err=%v accessCalls=%d", err, fake.accessCalls)
	}
}

func TestRunPreflightRequiresBothArchitectures(t *testing.T) {
	fake := &fakeCluster{server: "https://api.example", checks: []CheckResult{{Name: "pipelines", Passed: true}}}
	_, err := RunPreflight(context.Background(), fake, PreflightSpec{ExpectedServer: "https://api.example", RequireAMD64Capacity: true, RequireARM64Capacity: true, ArchitectureReady: map[string]bool{"amd64": true, "arm64": false}})
	if err == nil {
		t.Fatal("expected missing arm64 capacity error")
	}
}
