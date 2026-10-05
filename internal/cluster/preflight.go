package cluster

import (
	"context"
	"fmt"
	"strings"
)

type PreflightSpec struct {
	ExpectedServer string
	AccessChecks   []AccessCheck
	Scheduling     SchedulingSpec
}

type PreflightResult struct {
	Checks     []CheckResult `json:"checks"`
	Scheduling []CheckResult `json:"scheduling"`
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
	result := PreflightResult{}
	result.Checks = cluster.CheckAccess(ctx, spec.AccessChecks)
	for _, check := range result.Checks {
		if !check.Passed {
			return result, fmt.Errorf("preflight check %q failed: %s", check.Name, check.Details)
		}
	}
	if len(spec.Scheduling.Platforms) > 0 {
		result.Scheduling = cluster.CheckScheduling(ctx, spec.Scheduling)
		for _, check := range result.Scheduling {
			if !check.Passed {
				return result, fmt.Errorf("scheduling check %q failed: %s", check.Name, check.Details)
			}
		}
	}
	return result, nil
}
