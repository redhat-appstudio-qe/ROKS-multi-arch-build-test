package cluster

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type PreflightSpec struct {
	ExpectedServer       string
	AccessChecks         []AccessCheck
	ArchiveGVR           schema.GroupVersionResource
	RequireAMD64Capacity bool
	RequireARM64Capacity bool
	ArchitectureReady    map[string]bool
}

type PreflightResult struct {
	Checks       []CheckResult   `json:"checks"`
	Archive      ArchiveEndpoint `json:"archive"`
	Architecture map[string]bool `json:"architecture"`
}

func RunPreflight(ctx context.Context, cluster Cluster, spec PreflightSpec) (PreflightResult, error) {
	if cluster == nil {
		return PreflightResult{}, fmt.Errorf("cluster is required")
	}
	if strings.TrimSpace(spec.ExpectedServer) == "" {
		return PreflightResult{}, fmt.Errorf("expected cluster server is required")
	}
	actual, err := cluster.Server(ctx)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("read cluster server: %w", err)
	}
	if strings.TrimRight(actual, "/") != strings.TrimRight(spec.ExpectedServer, "/") {
		return PreflightResult{}, fmt.Errorf("cluster server mismatch: expected %q, got %q", spec.ExpectedServer, actual)
	}
	result := PreflightResult{Architecture: map[string]bool{}}
	result.Checks = cluster.CheckAccess(ctx, spec.AccessChecks)
	for _, check := range result.Checks {
		if !check.Passed {
			return result, fmt.Errorf("preflight check %q failed: %s", check.Name, check.Details)
		}
	}
	if spec.ArchiveGVR.Resource != "" {
		result.Archive, err = cluster.DiscoverArchive(ctx, spec.ArchiveGVR)
		if err != nil {
			return result, fmt.Errorf("discover archive: %w", err)
		}
	}
	for architecture, ready := range spec.ArchitectureReady {
		result.Architecture[architecture] = ready
	}
	if spec.RequireAMD64Capacity && !result.Architecture["amd64"] {
		return result, fmt.Errorf("amd64 capacity cannot be proven")
	}
	if spec.RequireARM64Capacity && !result.Architecture["arm64"] {
		return result, fmt.Errorf("arm64 capacity cannot be proven")
	}
	return result, nil
}
