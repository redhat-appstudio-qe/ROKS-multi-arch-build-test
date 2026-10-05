package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
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
	Prompt      Confirmation
	Namespace   cleanup.NamespaceService
}

func NewLiveStage(request config.Request, clients *cluster.ClientSet, provider providers.Provider) (*LiveStage, error) {
	source, err := providers.ParseRepository(request.SourceRepository)
	if err != nil {
		return nil, err
	}
	if clients == nil || provider == nil {
		return nil, fmt.Errorf("cluster clients and provider are required")
	}
	stage := &LiveStage{Request: request, Clients: clients, RealCluster: &cluster.RealCluster{Clients: clients}, Provider: provider, Source: source, Fixture: providers.FixtureRepository{Owner: source.Owner, Name: source.Name, URL: source.URL}, Namespace: cleanup.NamespaceService{Dynamic: clients.Dynamic}}
	return stage, nil
}

func (s *LiveStage) Preflight(ctx context.Context, manifest *model.RunManifest) error {
	spec := cluster.PreflightSpec{ExpectedServer: s.Request.ClusterServer, AccessChecks: []cluster.AccessCheck{
		{Name: "pipelineruns", Resource: PipelineRunGVR, Namespace: s.Request.TenantNamespace},
		{Name: "taskruns", Resource: cluster.TaskRunGVR, Namespace: s.Request.TenantNamespace},
		{Name: "applications", Resource: cluster.ApplicationGVR, Namespace: s.Request.TenantNamespace},
		{Name: "components", Resource: cluster.ComponentGVR, Namespace: s.Request.TenantNamespace},
	}, Scheduling: cluster.SchedulingSpec{
		Namespace:  cluster.MultiPlatformControllerNamespace,
		Deployment: cluster.MultiPlatformControllerDeployment,
		ConfigMap:  cluster.MultiPlatformControllerConfigMap,
		Platforms:  []string{"linux/amd64", "linux/arm64"},
	}}
	_, err := cluster.RunPreflight(ctx, s.RealCluster, spec)
	if err != nil {
		return err
	}
	if err := s.ensureTenantNamespaceAvailable(ctx, manifest); err != nil {
		return err
	}
	manifest.TargetClusterServer = s.Request.ClusterServer
	return nil
}

func (s *LiveStage) EnsureFixture(ctx context.Context, manifest *model.RunManifest) error {
	fixture := s.Fixture
	if recorded := fixtureFromManifest(manifest.Fixture); recorded.Name != "" {
		fixture = recorded
	}
	var err error
	fixture, err = s.Provider.ValidateFixture(ctx, fixture)
	if err != nil {
		return err
	}
	s.Fixture = fixture
	components := []cluster.ComponentSpec{
		{Name: "mathwizz-web-server", Repository: fixture.URL, Branch: s.Request.SourceBranch, Dockerfile: s.Request.ComponentPaths[0]},
		{Name: "mathwizz-history-worker", Repository: fixture.URL, Branch: s.Request.SourceBranch, Dockerfile: s.Request.ComponentPaths[1]},
		{Name: "mathwizz-frontend", Repository: fixture.URL, Branch: s.Request.SourceBranch, Dockerfile: s.Request.ComponentPaths[2]},
	}
	namespace := manifest.Fixture.TenantNamespace
	if namespace == "" {
		namespace = s.Request.TenantNamespace
	}
	application := manifest.Fixture.Application
	if application == "" {
		application = s.Request.ApplicationName
	}
	result, err := (cluster.FixtureService{Dynamic: s.Clients.Dynamic}).EnsureFixture(ctx, cluster.FixtureSpec{
		Namespace:   namespace,
		RunID:       manifest.RunID,
		Application: application,
		Provider:    s.Request.Provider,
		GitLabToken: s.Request.Credentials.GitLabToken,
		Components:  components,
	})
	if err != nil {
		return err
	}
	manifest.Fixture = model.FixtureIdentity{
		TenantNamespace: result.Namespace,
		Application:     result.Application,
		Components:      result.Components,
		RepositoryOwner: fixture.Owner,
		RepositoryName:  fixture.Name,
		Repository:      fixture.URL,
		Branch:          s.Request.SourceBranch,
	}
	return nil
}

func (s *LiveStage) ensureTenantNamespaceAvailable(ctx context.Context, manifest *model.RunManifest) error {
	namespace := manifest.Fixture.TenantNamespace
	if namespace == "" {
		namespace = s.Request.TenantNamespace
	}
	candidates, err := s.Namespace.FindCandidates(ctx, namespace)
	if err != nil {
		return fmt.Errorf("discover tenant namespace %s: %w", namespace, err)
	}
	for _, info := range candidates {
		if s.Request.ResumeRunID != "" && info.Name == namespace && info.RunID == manifest.RunID {
			continue
		}
		if s.Prompt == nil {
			return fmt.Errorf("tenant namespace %s from run %s requires interactive approval before deletion", info.Name, info.RunID)
		}
		approved, err := s.Prompt.Confirm(ctx, fmt.Sprintf("Delete owned stale tenant namespace %s from run %s", info.Name, info.RunID))
		if err != nil {
			return fmt.Errorf("confirm deletion of tenant namespace %s: %w", info.Name, err)
		}
		if !approved {
			return fmt.Errorf("tenant namespace %s retained; preflight refused to continue", info.Name)
		}
		if err := s.Namespace.Delete(ctx, info.Name, info.RunID); err != nil {
			return err
		}
		if err := s.Namespace.WaitDeleted(ctx, info.Name); err != nil {
			return err
		}
	}
	return nil
}

func (s *LiveStage) TriggerComponents(ctx context.Context, manifest *model.RunManifest) error {
	if s.Fixture.Name == "" {
		s.Fixture = fixtureFromManifest(manifest.Fixture)
	}
	if s.Fixture.Name == "" {
		return fmt.Errorf("run manifest does not contain a fixture repository")
	}
	paths := s.Request.ComponentPaths
	components := manifest.Fixture.Components
	if len(paths) != len(components) {
		return fmt.Errorf("fixture has %d components but %d Dockerfile paths", len(components), len(paths))
	}
	componentNames := make([]string, 0, len(components))
	for _, component := range components {
		componentNames = append(componentNames, component)
	}
	if err := waitForPaCEnabled(ctx, s.Request.Timeouts.Build, 10*time.Second, componentNames, func(ctx context.Context) ([]unstructured.Unstructured, error) {
		list, err := s.Clients.Dynamic.Resource(cluster.ComponentGVR).Namespace(manifest.Fixture.TenantNamespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list components while waiting for PaC: %w", err)
		}
		for index := range list.Items {
			if err := paCStatusError(&list.Items[index]); err != nil {
				return nil, err
			}
		}
		return list.Items, nil
	}); err != nil {
		return fmt.Errorf("wait for PaC onboarding: %w", err)
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

func fixtureFromManifest(identity model.FixtureIdentity) providers.FixtureRepository {
	return providers.FixtureRepository{
		Owner: identity.RepositoryOwner,
		Name:  identity.RepositoryName,
		URL:   identity.Repository,
	}
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

func waitForPaCEnabled(ctx context.Context, timeout, interval time.Duration, names []string, list func(context.Context) ([]unstructured.Unstructured, error)) error {
	if timeout <= 0 {
		return fmt.Errorf("PaC timeout must be positive")
	}
	if interval <= 0 {
		return fmt.Errorf("PaC polling interval must be positive")
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		objects, err := list(waitCtx)
		if err != nil {
			return err
		}
		byName := make(map[string]*unstructured.Unstructured, len(objects))
		for index := range objects {
			byName[objects[index].GetName()] = &objects[index]
		}
		pending := false
		for _, name := range names {
			object, found := byName[name]
			if !found {
				pending = true
				continue
			}
			status := object.GetAnnotations()["build.appstudio.openshift.io/status"]
			var annotation struct {
				PaC struct {
					State string `json:"state"`
				} `json:"pac"`
			}
			if err := json.Unmarshal([]byte(status), &annotation); err != nil || annotation.PaC.State != "enabled" {
				pending = true
			}
		}
		if !pending {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return fmt.Errorf("timed out waiting for PaC onboarding: %w", waitCtx.Err())
		case <-timer.C:
		}
	}
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

func (s *LiveStage) VerifyBuildOutputs(ctx context.Context, manifest *model.RunManifest) error {
	evidence, err := (cluster.BuildOutputInspector{Dynamic: s.Clients.Dynamic}).Verify(ctx, manifest.PipelineRuns)
	if err != nil {
		return err
	}
	manifest.BuildOutputs = append(manifest.BuildOutputs, evidence...)
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
