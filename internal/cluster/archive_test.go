package cluster

import (
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/types"
)

func TestCompareArchiveIdentityRequiresUID(t *testing.T) {
	comparisons, err := CompareArchiveIdentity(map[string]any{"metadata": map[string]any{"namespace": "tenant", "name": "run", "uid": "uid-1"}}, model.PipelineRunIdentity{Namespace: "tenant", Name: "run", UID: types.UID("uid-2")})
	if err != nil {
		t.Fatal(err)
	}
	if comparisons["namespace"] != true || comparisons["name"] != true || comparisons["uid"] != false {
		t.Fatalf("unexpected comparisons: %#v", comparisons)
	}
}
