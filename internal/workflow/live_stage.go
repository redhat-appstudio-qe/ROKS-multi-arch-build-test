package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/redhat-appstudio/konflux-test/internal/cluster"
	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var PipelineRunGVR = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}

type LiveStage struct {
	Request     config.Request
	Clients     *cluster.ClientSet
	RealCluster *cluster.RealCluster
	Provider    providers.Provider
	Source      providers.SourceRepository
	Fixture     providers.FixtureRepository
	Archive     cluster.ArchiveEndpoint
}

func NewLiveStage(request config.Request, clients *cluster.ClientSet, provider providers.Provider) (*LiveStage, error) {
	source, err := providers.ParseRepository(request.SourceRepository)
	if err != nil {
		return nil, err
	}
	if clients == nil || provider == nil {
		return nil, fmt.Errorf("cluster clients and provider are required")
	}
	return &LiveStage{Request: request, Clients: clients, RealCluster: &cluster.RealCluster{Clients: clients}, Provider: provider, Source: source}, nil
}

func (s *LiveStage) Preflight(ctx context.Context, manifest *model.RunManifest) error {
	spec := cluster.PreflightSpec{ExpectedServer: s.Request.ClusterServer, AccessChecks: []cluster.AccessCheck{
		{Name: "pipelineruns", Resource: PipelineRunGVR, Namespace: s.Request.TenantNamespace},
		{Name: "applications", Resource: cluster.ApplicationGVR, Namespace: s.Request.TenantNamespace},
		{Name: "components", Resource: cluster.ComponentGVR, Namespace: s.Request.TenantNamespace},
	}}
	if s.Request.ArchiveGVR != "" {
		gvr, err := cluster.ParseGVR(s.Request.ArchiveGVR)
		if err != nil {
			return err
		}
		spec.ArchiveGVR = gvr
	}
	result, err := cluster.RunPreflight(ctx, s.RealCluster, spec)
	if err != nil {
		return err
	}
	s.Archive = result.Archive
	manifest.TargetClusterServer = s.Request.ClusterServer
	return nil
}

func (s *LiveStage) EnsureFixture(ctx context.Context, manifest *model.RunManifest) error {
	fixture, err := s.Provider.EnsureFork(ctx, s.Source, s.Fixture)
	if err != nil {
		return err
	}
	s.Fixture = fixture
	components := []cluster.ComponentSpec{
		{Name: "mathwizz-web-server", Repository: fixture.URL, Branch: s.Request.SourceBranch, Dockerfile: s.Request.ComponentPaths[0]},
		{Name: "mathwizz-history-worker", Repository: fixture.URL, Branch: s.Request.SourceBranch, Dockerfile: s.Request.ComponentPaths[1]},
		{Name: "mathwizz-frontend", Repository: fixture.URL, Branch: s.Request.SourceBranch, Dockerfile: s.Request.ComponentPaths[2]},
	}
	result, err := (cluster.FixtureService{Dynamic: s.Clients.Dynamic}).EnsureFixture(ctx, cluster.FixtureSpec{Namespace: s.Request.TenantNamespace, Application: s.Request.ApplicationName, Components: components})
	if err != nil {
		return err
	}
	manifest.Fixture = model.FixtureIdentity{TenantNamespace: result.Namespace, Application: result.Application, Components: result.Components, Repository: fixture.URL, Branch: s.Request.SourceBranch}
	return nil
}

func (s *LiveStage) CaptureBaseline(ctx context.Context, manifest *model.RunManifest) error {
	list, err := s.Clients.Dynamic.Resource(PipelineRunGVR).Namespace(s.Request.TenantNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, object := range list.Items {
		manifest.BaselinePipelineRuns = append(manifest.BaselinePipelineRuns, identityFromObject(&object, "", ""))
	}
	return nil
}

func (s *LiveStage) TriggerComponents(ctx context.Context, manifest *model.RunManifest) error {
	paths := s.Request.ComponentPaths
	components := manifest.Fixture.Components
	if len(paths) != len(components) {
		return fmt.Errorf("fixture has %d components but %d Dockerfile paths", len(components), len(paths))
	}
	for index, component := range components {
		file, err := s.Provider.ReadFile(ctx, s.Fixture, paths[index], s.Request.SourceBranch)
		if err != nil {
			return fmt.Errorf("read %s: %w", paths[index], err)
		}
		comment := fmt.Sprintf(TriggerCommentFormat, manifest.RunID, time.Now().Unix())
		content := strings.TrimRight(file.Content, "\n") + "\n" + comment + "\n"
		commit, err := s.Provider.UpdateFile(ctx, s.Fixture, paths[index], s.Request.SourceBranch, content, file.SHA)
		if err != nil {
			return fmt.Errorf("trigger %s: %w", component, err)
		}
		manifest.TriggerCommits = append(manifest.TriggerCommits, model.TriggerCommit{Component: component, SHA: commit.SHA, URL: commit.URL, CreatedAt: commit.CreatedAt})
	}
	return nil
}

func (s *LiveStage) VerifyBuilds(ctx context.Context, manifest *model.RunManifest) error {
	identities, err := waitForBuildMatches(ctx, s.Request.Timeouts.Build, 10*time.Second, manifest.TriggerCommits, func(ctx context.Context) ([]unstructured.Unstructured, error) {
		components, err := s.Clients.Dynamic.Resource(cluster.ComponentGVR).Namespace(s.Request.TenantNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list components while waiting for builds: %w", err)
		}
		for index := range components.Items {
			if err := paCStatusError(&components.Items[index]); err != nil {
				return nil, err
			}
		}
		list, err := s.Clients.Dynamic.Resource(PipelineRunGVR).Namespace(s.Request.TenantNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
	if err != nil {
		return err
	}
	manifest.PipelineRuns = append(manifest.PipelineRuns, identities...)
	return nil
}

func paCStatusError(object *unstructured.Unstructured) error {
	if object == nil {
		return nil
	}
	annotation := object.GetAnnotations()["build.appstudio.openshift.io/status"]
	if annotation == "" {
		return nil
	}
	var status struct {
		PaC struct {
			State        string `json:"state"`
			ErrorID      int    `json:"error-id"`
			ErrorMessage string `json:"error-message"`
		} `json:"pac"`
	}
	if err := json.Unmarshal([]byte(annotation), &status); err != nil || status.PaC.State != "error" {
		return nil
	}
	if status.PaC.ErrorMessage == "" {
		return fmt.Errorf("component %s PaC onboarding failed with error ID %d", object.GetName(), status.PaC.ErrorID)
	}
	return fmt.Errorf("component %s PaC onboarding failed: %s", object.GetName(), status.PaC.ErrorMessage)
}

func waitForBuildMatches(ctx context.Context, timeout, interval time.Duration, commits []model.TriggerCommit, list func(context.Context) ([]unstructured.Unstructured, error)) ([]model.PipelineRunIdentity, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("build timeout must be positive")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("build polling interval must be positive")
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		identities, pending, err := matchBuilds(waitCtx, commits, list)
		if err != nil {
			return nil, err
		}
		if !pending {
			return identities, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return nil, fmt.Errorf("timed out waiting for PipelineRuns: %w", waitCtx.Err())
		case <-timer.C:
		}
	}
}

func matchBuilds(ctx context.Context, commits []model.TriggerCommit, list func(context.Context) ([]unstructured.Unstructured, error)) ([]model.PipelineRunIdentity, bool, error) {
	objects, err := list(ctx)
	if err != nil {
		return nil, false, err
	}
	identities := make([]model.PipelineRunIdentity, 0, len(commits))
	for _, commit := range commits {
		found := false
		pending := false
		for index := range objects {
			object := &objects[index]
			labels := object.GetLabels()
			if labels["pipelinesascode.tekton.dev/sha"] != commit.SHA || labels["appstudio.openshift.io/component"] != commit.Component {
				continue
			}
			found = true
			status := pipelineRunConditionStatus(object)
			switch status {
			case "True":
				identities = append(identities, identityFromObject(object, commit.Component, commit.SHA))
				pending = false
				goto nextCommit
			case "False":
				return nil, false, fmt.Errorf("PipelineRun %s/%s failed", object.GetNamespace(), object.GetName())
			default:
				pending = true
			}
		}
		if !found || pending {
			return identities, true, nil
		}
	nextCommit:
	}
	return identities, false, nil
}

func pipelineRunConditionStatus(object *unstructured.Unstructured) string {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok || condition["type"] != "Succeeded" {
			continue
		}
		status, _ := condition["status"].(string)
		return status
	}
	return "Unknown"
}

func (s *LiveStage) VerifyImages(ctx context.Context, manifest *model.RunManifest) error {
	builds := make([]cluster.BuildImage, 0, len(manifest.PipelineRuns))
	for _, identity := range manifest.PipelineRuns {
		object, err := s.Clients.Dynamic.Resource(PipelineRunGVR).Namespace(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		reference, digest := imageResults(object)
		if reference == "" || digest == "" {
			return fmt.Errorf("PipelineRun %s/%s has no image reference and digest results", identity.Namespace, identity.Name)
		}
		builds = append(builds, cluster.BuildImage{Component: identity.Component, Reference: reference, Digest: digest})
	}
	images, err := cluster.ValidateImages(ctx, registryInspector{}, builds)
	if err != nil {
		return err
	}
	manifest.Images = append(manifest.Images, images...)
	return nil
}

func (s *LiveStage) ObservePruning(ctx context.Context, manifest *model.RunManifest) error {
	for _, identity := range manifest.PipelineRuns {
		started := time.Now().UTC()
		err := cluster.WaitForPruned(ctx, func(ctx context.Context, identity model.PipelineRunIdentity) error {
			_, err := s.Clients.Dynamic.Resource(PipelineRunGVR).Namespace(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
			return err
		}, identity, s.Request.Timeouts.Pruning, 10*time.Second)
		if err != nil {
			return err
		}
		manifest.Pruning = append(manifest.Pruning, model.PruningObservation{PipelineRun: identity, ObservedAt: started, DisappearedAt: time.Now().UTC(), Normal: true})
	}
	return nil
}

func (s *LiveStage) VerifyArchive(ctx context.Context, manifest *model.RunManifest) error {
	if s.Archive.GVR.Resource == "" {
		return fmt.Errorf("archive endpoint was not discovered")
	}
	for _, identity := range manifest.PipelineRuns {
		raw, err := cluster.QueryArchive(ctx, s.Clients.Dynamic, s.Archive, identity)
		if err != nil {
			return err
		}
		comparisons, err := cluster.CompareArchiveIdentity(raw, identity)
		if err != nil {
			return err
		}
		matched := comparisons["namespace"] && comparisons["name"] && comparisons["uid"]
		manifest.Archive = append(manifest.Archive, model.ArchiveEvidence{Endpoint: s.Archive.APIURL, QueriedAt: time.Now().UTC(), RawResponse: raw, Matched: matched, Comparisons: comparisons})
		if !matched {
			return fmt.Errorf("archive identity mismatch for %s/%s", identity.Namespace, identity.Name)
		}
	}
	return nil
}

func identityFromObject(object *unstructured.Unstructured, component, sourceSHA string) model.PipelineRunIdentity {
	identity := model.PipelineRunIdentity{Namespace: object.GetNamespace(), Name: object.GetName(), UID: object.GetUID(), Component: component, SourceSHA: sourceSHA, StartedAt: object.GetCreationTimestamp().Time}
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok || condition["type"] != "Succeeded" {
			continue
		}
		identity.Succeeded = condition["status"] == "True"
		if value, ok := condition["lastTransitionTime"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				identity.CompletedAt = parsed
			}
		}
	}
	return identity
}

func imageResults(object *unstructured.Unstructured) (string, string) {
	results, _, _ := unstructured.NestedSlice(object.Object, "status", "results")
	var reference, digest string
	for _, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := result["name"].(string)
		value, _ := result["value"].(string)
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "IMAGE") && (strings.Contains(upper, "URL") || strings.Contains(upper, "REFERENCE")) {
			reference = value
		}
		if strings.Contains(upper, "IMAGE") && strings.Contains(upper, "DIGEST") {
			digest = value
		}
	}
	return reference, digest
}

type registryInspector struct{}

func (registryInspector) Inspect(ctx context.Context, reference string) ([]cluster.ImagePlatform, error) {
	parsed, err := name.ParseReference(reference)
	if err != nil {
		return nil, err
	}
	descriptor, err := remote.Get(parsed, remote.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if descriptor.MediaType.IsIndex() {
		index, err := descriptor.ImageIndex()
		if err == nil {
			manifest, err := index.IndexManifest()
			if err == nil {
				platforms := make([]cluster.ImagePlatform, 0, len(manifest.Manifests))
				for _, entry := range manifest.Manifests {
					if entry.Platform == nil {
						continue
					}
					platforms = append(platforms, cluster.ImagePlatform{OS: entry.Platform.OS, Architecture: entry.Platform.Architecture, Digest: entry.Digest.String()})
				}
				return platforms, nil
			}
		}
	}
	image, err := descriptor.Image()
	if err != nil {
		return nil, err
	}
	config, err := image.ConfigFile()
	if err != nil {
		return nil, err
	}
	digest := descriptor.Descriptor.Digest.String()
	return []cluster.ImagePlatform{{OS: config.OS, Architecture: config.Architecture, Digest: digest}}, nil
}
