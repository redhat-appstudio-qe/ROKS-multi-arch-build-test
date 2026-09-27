package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeInspector struct{ platforms []ImagePlatform }

func (f fakeInspector) Inspect(context.Context, string) ([]ImagePlatform, error) {
	return f.platforms, nil
}

func TestValidateImagesRequiresBothArchitectures(t *testing.T) {
	got, err := ValidateImages(context.Background(), fakeInspector{platforms: []ImagePlatform{{OS: "linux", Architecture: "amd64", Digest: "sha256:build"}}}, []BuildImage{{Component: "web", Reference: "quay.io/example/web", Digest: "sha256:build"}})
	if err == nil || got != nil {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	got, err = ValidateImages(context.Background(), fakeInspector{platforms: []ImagePlatform{{OS: "linux", Architecture: "amd64", Digest: "sha256:build"}, {OS: "linux", Architecture: "arm64", Digest: "sha256:build"}}}, []BuildImage{{Component: "web", Reference: "quay.io/example/web", Digest: "sha256:build"}})
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestWaitForPrunedAcceptsNormalNotFound(t *testing.T) {
	identity := model.PipelineRunIdentity{Namespace: "tenant", Name: "run"}
	seen := 0
	err := WaitForPruned(context.Background(), func(context.Context, model.PipelineRunIdentity) error {
		seen++
		return apierrors.NewNotFound(schema.GroupResource{Group: "tekton.dev", Resource: "pipelineruns"}, identity.Name)
	}, identity, time.Second, time.Millisecond)
	if err != nil || seen != 1 {
		t.Fatalf("seen=%d err=%v", seen, err)
	}
}
