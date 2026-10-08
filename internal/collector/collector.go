package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"sigs.k8s.io/yaml"
)

var requiredArtifactNames = []string{
	"session/manifest.json",
	"workload/applications.json",
	"workload/components.json",
	"workload/pipelineruns.json",
	"workload/taskruns.json",
	"workload/taskruns/",
	"workload/pods.json",
	"collection-report.json",
}

type Source struct {
	Name     string
	Path     string
	Optional bool
	Collect  func(context.Context, string) error
}

type Attempt struct {
	Name      string    `json:"name"`
	Path      string    `json:"path,omitempty"`
	Optional  bool      `json:"optional,omitempty"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type Report struct {
	RunID     string    `json:"runID"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
	Attempts  []Attempt `json:"attempts"`
}

type Collector struct {
	StateDir string
	Now      func() time.Time
}

func RequiredArtifactNames() []string {
	return append([]string(nil), requiredArtifactNames...)
}

func RequiredArtifactNamesForManifest(manifest model.RunManifest) []string {
	names := RequiredArtifactNames()
	if len(manifest.FailedBuildLogs) > 0 {
		names = append(names, "failed-builds/")
	}
	return names
}

func (c Collector) Collect(ctx context.Context, runID string, manifest model.RunManifest, sources []Source) (Report, error) {
	if c.StateDir == "" || runID == "" {
		return Report{}, fmt.Errorf("state directory and run ID are required")
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	root := c.runRoot(runID, manifest)
	if err := os.MkdirAll(filepath.Join(root, "workload"), 0o750); err != nil {
		return Report{}, fmt.Errorf("create collection directory: %w", err)
	}
	if err := writeJSON(filepath.Join(root, "session", "manifest.json"), manifest); err != nil {
		return Report{}, err
	}
	report := Report{RunID: runID, StartedAt: now().UTC(), Attempts: []Attempt{{Name: "manifest", Path: "session/manifest.json", Status: "success", StartedAt: now().UTC(), EndedAt: now().UTC()}}}
	for _, source := range sources {
		attempt := Attempt{Name: source.Name, Path: source.Path, Optional: source.Optional, StartedAt: now().UTC()}
		if source.Collect == nil {
			attempt.Status = "omitted"
		} else if err := source.Collect(ctx, root); err != nil {
			attempt.Status = "error"
			attempt.Error = err.Error()
		} else {
			attempt.Status = "success"
		}
		attempt.EndedAt = now().UTC()
		report.Attempts = append(report.Attempts, attempt)
	}
	if len(manifest.FailedBuildLogs) > 0 && !successfulAttempt(report, "failed-builds/") {
		failedBuildsPath := filepath.Join(root, "failed-builds")
		if info, err := os.Stat(failedBuildsPath); err == nil && info.IsDir() {
			report.Attempts = append(report.Attempts, Attempt{Name: "failed-builds", Path: "failed-builds/", Status: "success", StartedAt: now().UTC(), EndedAt: now().UTC()})
		}
	}
	report.EndedAt = now().UTC()
	report.Attempts = append(report.Attempts, Attempt{Name: "collection-report", Path: "collection-report.json", Status: "success", StartedAt: report.EndedAt, EndedAt: report.EndedAt})
	if err := writeJSON(filepath.Join(root, "collection-report.json"), report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (c Collector) CollectAndVerify(ctx context.Context, runID string, manifest model.RunManifest, sources []Source) (model.FailureArtifactReport, error) {
	artifactReport := c.emptyArtifactReport(runID, manifest)
	_, err := c.Collect(ctx, runID, manifest, sources)
	if err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, err.Error())
		return artifactReport, err
	}
	return c.VerifySavedArtifacts(runID, manifest)
}

// VerifySavedArtifacts checks the existing collection report and all required
// artifacts without making another request to the cluster.
func (c Collector) VerifySavedArtifacts(runID string, manifest model.RunManifest) (model.FailureArtifactReport, error) {
	artifactReport := c.emptyArtifactReport(runID, manifest)
	if c.StateDir == "" || runID == "" {
		return artifactReport, fmt.Errorf("state directory and run ID are required")
	}
	if manifest.RunID != runID {
		return artifactReport, fmt.Errorf("run identity mismatch: requested %q, found %q", runID, manifest.RunID)
	}
	artifactDirectory := manifest.ArtifactDirectory
	if strings.TrimSpace(artifactDirectory) == "" {
		artifactDirectory = runID
	}
	if err := validateArtifactDirectory(artifactDirectory); err != nil {
		return artifactReport, err
	}
	root := filepath.Join(c.StateDir, artifactDirectory)
	artifactReport.ArtifactPath = root
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = fmt.Errorf("run artifact path is not a directory")
		}
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, "run artifact directory: "+err.Error())
		return artifactReport, fmt.Errorf("read saved artifacts: %w", err)
	}
	reportData, err := readRegularFile(filepath.Join(root, "collection-report.json"))
	if err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, "collection-report.json: "+err.Error())
		return artifactReport, fmt.Errorf("read collection report: %w", err)
	}
	var storedReport Report
	if err := json.Unmarshal(reportData, &storedReport); err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, "collection-report.json: "+err.Error())
		return artifactReport, fmt.Errorf("decode collection report: %w", err)
	}
	if storedReport.RunID != runID {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, fmt.Sprintf("collection-report.json: run ID %q does not match %q", storedReport.RunID, runID))
	}
	for _, attempt := range storedReport.Attempts {
		if attempt.Status == "error" && !attempt.Optional {
			artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, attempt.Name+": "+attempt.Error)
		}
	}
	manifestRelativePath := filepath.Join("session", "manifest.json")
	manifestData, manifestErr := readRegularFileWithinRoot(root, manifestRelativePath)
	if manifestErr == nil {
		var savedManifest model.RunManifest
		if decodeErr := json.Unmarshal(manifestData, &savedManifest); decodeErr != nil {
			manifestErr = fmt.Errorf("decode manifest: %w", decodeErr)
		} else if savedManifest.RunID != runID || savedManifest.Provider != manifest.Provider || savedManifest.TargetClusterServer != manifest.TargetClusterServer || savedManifest.Fixture.TenantNamespace != manifest.Fixture.TenantNamespace || (manifest.ArtifactDirectory != "" && savedManifest.ArtifactDirectory != manifest.ArtifactDirectory) {
			manifestErr = fmt.Errorf("saved manifest identity does not match run %q", runID)
		}
	}
	if manifestErr != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, "session/manifest.json: "+manifestErr.Error())
	}
	for _, name := range artifactReport.RequiredArtifactNames {
		path := filepath.Join(root, name)
		var readErr error
		if strings.HasSuffix(name, "/") {
			info, statErr := os.Lstat(path)
			if statErr != nil {
				readErr = statErr
			} else if info.Mode()&os.ModeSymlink != 0 {
				readErr = fmt.Errorf("artifact path is a symbolic link")
			} else if !info.IsDir() {
				readErr = fmt.Errorf("not a directory")
			}
		} else {
			_, readErr = readRegularFileWithinRoot(root, name)
		}
		if readErr != nil {
			artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, name+": "+readErr.Error())
			continue
		}
		if name != "session/manifest.json" && !successfulAttempt(storedReport, name) {
			artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, name+": collection was not successful")
			continue
		}
		artifactReport.SavedArtifactNames = append(artifactReport.SavedArtifactNames, name)
	}
	if err := verifyTaskRunSnapshots(root, manifest.Fixture.TenantNamespace); err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, err.Error())
	}
	if err := verifyFailedBuildLogs(root, manifest.FailedBuildLogs); err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, err.Error())
	}
	if len(artifactReport.CollectionErrors) > 0 {
		return artifactReport, fmt.Errorf("saved artifacts incomplete under %s: %v", root, artifactReport.CollectionErrors)
	}
	artifactReport.VerifiedAt = time.Now().UTC()
	return artifactReport, nil
}

func verifyTaskRunSnapshots(root, namespace string) error {
	data, err := readRegularFileWithinRoot(root, filepath.Join("workload", "taskruns.json"))
	if err != nil {
		return fmt.Errorf("workload/taskruns.json: %w", err)
	}
	var list struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("decode workload/taskruns.json: %w", err)
	}
	if len(list.Items) == 0 || strings.TrimSpace(string(list.Items)) == "null" {
		return fmt.Errorf("workload/taskruns.json: items must be an array")
	}
	var items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(list.Items, &items); err != nil {
		return fmt.Errorf("decode workload/taskruns.json items: %w", err)
	}
	for _, item := range items {
		if strings.TrimSpace(item.Metadata.Name) == "" {
			return fmt.Errorf("workload/taskruns.json: TaskRun item has no metadata.name")
		}
		if strings.TrimSpace(namespace) == "" {
			return fmt.Errorf("workload/taskruns.json: cannot verify TaskRun %q without a fixture namespace", item.Metadata.Name)
		}
		name := safePathPart(namespace+"--"+item.Metadata.Name) + ".yaml"
		relativePath := filepath.Join("workload", "taskruns", name)
		snapshotData, err := readRegularFileWithinRoot(root, relativePath)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.ToSlash(relativePath), err)
		}
		if strings.TrimSpace(string(snapshotData)) == "" {
			return fmt.Errorf("%s: TaskRun snapshot is empty", filepath.ToSlash(relativePath))
		}
		var snapshot struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal(snapshotData, &snapshot); err != nil {
			return fmt.Errorf("%s: decode TaskRun snapshot: %w", filepath.ToSlash(relativePath), err)
		}
		if snapshot.Kind != "TaskRun" || snapshot.Metadata.Name != item.Metadata.Name || snapshot.Metadata.Namespace != namespace {
			return fmt.Errorf("%s: TaskRun snapshot identity mismatch: expected kind TaskRun, metadata.name %q, metadata.namespace %q; found kind %q, metadata.name %q, metadata.namespace %q", filepath.ToSlash(relativePath), item.Metadata.Name, namespace, snapshot.Kind, snapshot.Metadata.Name, snapshot.Metadata.Namespace)
		}
	}
	return nil
}

func verifyFailedBuildLogs(root string, logs []model.FailedBuildLog) error {
	for _, log := range logs {
		if strings.TrimSpace(log.LogPath) == "" {
			continue
		}
		relativePath, err := artifactRelativePath(root, log.LogPath)
		if err != nil {
			return fmt.Errorf("failed-build log path %q: %w", log.LogPath, err)
		}
		if filepath.Dir(relativePath) != "failed-builds" {
			return fmt.Errorf("failed-build log path %q is outside failed-builds/", log.LogPath)
		}
		if _, err := readRegularFileWithinRoot(root, relativePath); err != nil {
			return fmt.Errorf("%s: %w", filepath.ToSlash(relativePath), err)
		}
	}
	return nil
}

func artifactRelativePath(root, path string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		workingDirectoryPath, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		rootRelativePath, err := filepath.Abs(filepath.Join(rootAbs, path))
		if err != nil {
			return "", err
		}
		cleanPath := filepath.Clean(path)
		if filepath.Dir(cleanPath) == "failed-builds" {
			candidates = []string{rootRelativePath, workingDirectoryPath}
		} else {
			candidates = []string{workingDirectoryPath, rootRelativePath}
		}
	}
	for _, candidate := range candidates {
		relativePath, err := filepath.Rel(rootAbs, candidate)
		if err != nil {
			continue
		}
		if relativePath != "." && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return relativePath, nil
		}
	}
	return "", fmt.Errorf("path does not resolve inside the run artifact directory")
}

func readRegularFileWithinRoot(root, relativePath string) ([]byte, error) {
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("artifact path does not resolve inside the run artifact directory")
	}
	current := root
	parts := strings.Split(cleanPath, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("artifact path contains a symbolic link")
		}
		if index < len(parts)-1 {
			if !info.IsDir() {
				return nil, fmt.Errorf("artifact path parent is not a directory")
			}
		} else if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("artifact path is not a regular file")
		}
	}
	return os.ReadFile(current)
}

func (c Collector) emptyArtifactReport(runID string, manifest model.RunManifest) model.FailureArtifactReport {
	return model.FailureArtifactReport{ArtifactPath: c.runRoot(runID, manifest), RequiredArtifactNames: RequiredArtifactNamesForManifest(manifest)}
}

func validateArtifactDirectory(directory string) error {
	if strings.TrimSpace(directory) == "" || directory == "." || directory == ".." || filepath.IsAbs(directory) || filepath.Base(directory) != directory || strings.ContainsAny(directory, `/\\`) {
		return fmt.Errorf("artifact directory must be a single directory name")
	}
	return nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("artifact path is a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact path is not a regular file")
	}
	return os.ReadFile(path)
}

func (c Collector) runRoot(runID string, manifest model.RunManifest) string {
	directory := manifest.ArtifactDirectory
	if directory == "" {
		directory = runID
	}
	return filepath.Join(c.StateDir, directory)
}

func successfulAttempt(report Report, path string) bool {
	for _, attempt := range report.Attempts {
		if attempt.Path == path && attempt.Status == "success" {
			return true
		}
	}
	return false
}

func writeJSON(path string, value any) error {
	data, err := evidence.RedactedJSON(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

func ReadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}
