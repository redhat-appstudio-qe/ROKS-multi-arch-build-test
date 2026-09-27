package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type ImagePlatform struct {
	OS           string
	Architecture string
	Digest       string
}

type BuildImage struct {
	Component string
	Reference string
	Digest    string
}

type ImageInspector interface {
	Inspect(context.Context, string) ([]ImagePlatform, error)
}

func ValidateImages(ctx context.Context, inspector ImageInspector, builds []BuildImage) ([]model.ImageEvidence, error) {
	if inspector == nil {
		return nil, fmt.Errorf("image inspector is required")
	}
	result := make([]model.ImageEvidence, 0, len(builds)*2)
	for _, build := range builds {
		if build.Component == "" || build.Reference == "" || build.Digest == "" {
			return nil, fmt.Errorf("build image evidence is incomplete for %s", build.Component)
		}
		platforms, err := inspector.Inspect(ctx, build.Reference)
		if err != nil {
			return nil, fmt.Errorf("inspect image %s: %w", build.Reference, err)
		}
		found := map[string]bool{}
		for _, platform := range platforms {
			if platform.OS != "linux" || (platform.Architecture != "amd64" && platform.Architecture != "arm64") || platform.Digest == "" {
				continue
			}
			if platform.Digest == build.Digest {
				found[platform.Architecture] = true
				result = append(result, model.ImageEvidence{Component: build.Component, Reference: build.Reference, Digest: platform.Digest, OS: platform.OS, Architecture: platform.Architecture})
			}
		}
		if !found["amd64"] || !found["arm64"] {
			return nil, fmt.Errorf("image %s does not prove linux/amd64 and linux/arm64", build.Component)
		}
	}
	return result, nil
}

type PipelineRunGetter func(context.Context, model.PipelineRunIdentity) error

func WaitForPruned(ctx context.Context, getter PipelineRunGetter, identity model.PipelineRunIdentity, timeout, interval time.Duration) error {
	if getter == nil {
		return fmt.Errorf("pipeline run getter is required")
	}
	if timeout <= 0 || interval <= 0 {
		return fmt.Errorf("timeout and interval must be positive")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		err := getter(ctx, identity)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("pipeline run %s/%s did not disappear before pruning timeout", identity.Namespace, identity.Name)
		case <-ticker.C:
		}
	}
}
