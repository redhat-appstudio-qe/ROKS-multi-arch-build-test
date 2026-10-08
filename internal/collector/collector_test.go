package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/model"
)

var _ = ginkgo.Describe("Collector", func() {
	ginkgo.It("verifies required failure artifacts", func() {
		root := ginkgo.GinkgoT().TempDir()
		report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-1", model.RunManifest{RunID: "run-1"}, requiredTestSources())
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(report.ArtifactPath).To(Equal(filepath.Join(root, "run-1")))
		Expect(report.RequiredArtifactNames).To(HaveLen(8))
		Expect(report.SavedArtifactNames).To(HaveLen(8))
		Expect(report.VerifiedAt).NotTo(BeZero())
	})

	ginkgo.It("rejects a saved manifest reached through a symlinked session directory", func() {
		stateDir := ginkgo.GinkgoT().TempDir()
		collector := Collector{StateDir: stateDir}
		manifest := model.RunManifest{RunID: "run-session-symlink", Provider: "github", TargetClusterServer: "https://cluster.example", Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
		_, err := collector.Collect(context.Background(), manifest.RunID, manifest, requiredTestSources())
		Expect(err).ShouldNot(HaveOccurred())

		runRoot := collector.runRoot(manifest.RunID, manifest)
		savedManifest, err := os.ReadFile(filepath.Join(runRoot, "session", "manifest.json"))
		Expect(err).ShouldNot(HaveOccurred())
		externalSession := filepath.Join(ginkgo.GinkgoT().TempDir(), "session")
		Expect(os.MkdirAll(externalSession, 0o750)).Should(Succeed())
		Expect(os.WriteFile(filepath.Join(externalSession, "manifest.json"), savedManifest, 0o640)).Should(Succeed())
		Expect(os.RemoveAll(filepath.Join(runRoot, "session"))).Should(Succeed())
		Expect(os.Symlink(externalSession, filepath.Join(runRoot, "session"))).Should(Succeed())

		_, err = collector.VerifySavedArtifacts(manifest.RunID, manifest)
		Expect(err).Should(MatchError(ContainSubstring("symbolic link")))
	})

	ginkgo.It("suppresses verified report when required source fails", func() {
		root := ginkgo.GinkgoT().TempDir()
		sources := requiredTestSources()
		sources[0].Collect = func(context.Context, string) error { return errors.New("api unavailable") }
		report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-2", model.RunManifest{RunID: "run-2"}, sources)
		Expect(err).To(HaveOccurred())
		Expect(report.CollectionErrors).NotTo(BeEmpty())
		Expect(len(report.SavedArtifactNames)).To(BeNumerically("<", len(report.RequiredArtifactNames)))
	})

	ginkgo.It("redacts manifest values", func() {
		root := ginkgo.GinkgoT().TempDir()
		_, err := (Collector{StateDir: root}).Collect(context.Background(), "run-3", model.RunManifest{RunID: "run-3", ArtifactDirectory: "github-run-14:23_5.10.2026", BuildOutputs: []model.BuildOutputEvidence{{CreatedOutputs: map[string]string{"password": "secret-value"}}}}, nil)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		data, err := os.ReadFile(filepath.Join(root, "github-run-14:23_5.10.2026", "session", "manifest.json"))
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(string(data)).NotTo(ContainSubstring("secret-value"))
	})

	ginkgo.It("requires failed-build directory when failures exist", func() {
		root := ginkgo.GinkgoT().TempDir()
		sources := requiredTestSources()
		sources = append(sources, Source{Name: "failed-builds", Path: "failed-builds/", Collect: func(_ context.Context, root string) error {
			if err := os.MkdirAll(filepath.Join(root, "failed-builds"), 0o750); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "failed-builds", "build.log"), []byte("failed"), 0o640)
		}})
		manifest := model.RunManifest{
			RunID: "run-4",
			FailedBuildLogs: []model.FailedBuildLog{{
				PipelineRunName: "pr-1",
				TaskRunName:     "tr-1",
			}},
		}

		report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-4", manifest, sources)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(report.RequiredArtifactNames).To(HaveLen(9))
		Expect(report.SavedArtifactNames).To(HaveLen(9))
	})

	ginkgo.It("requires a YAML snapshot for every TaskRun in the saved list", func() {
		root := ginkgo.GinkgoT().TempDir()
		sources := requiredTestSources()
		for index := range sources {
			if sources[index].Path != "workload/taskruns.json" {
				continue
			}
			sources[index].Collect = func(_ context.Context, root string) error {
				return writeJSON(filepath.Join(root, "workload", "taskruns.json"), map[string]any{
					"items": []map[string]any{{"metadata": map[string]any{"name": "build-task"}}},
				})
			}
		}
		manifest := model.RunManifest{RunID: "run-5", Fixture: model.FixtureIdentity{TenantNamespace: "tenant-5"}}

		_, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), manifest.RunID, manifest, sources)
		Expect(err).To(MatchError(ContainSubstring("workload/taskruns/tenant-5--build-task.yaml")))
	})

	ginkgo.It("accepts a TaskRun snapshot at the collector's canonical filename", func() {
		root := ginkgo.GinkgoT().TempDir()
		sources := requiredTestSources()
		for index := range sources {
			switch sources[index].Path {
			case "workload/taskruns.json":
				sources[index].Collect = func(_ context.Context, root string) error {
					return writeJSON(filepath.Join(root, "workload", "taskruns.json"), map[string]any{
						"items": []map[string]any{{"metadata": map[string]any{"name": "build-task"}}},
					})
				}
			case "workload/taskruns/":
				sources[index].Collect = func(_ context.Context, root string) error {
					path := filepath.Join(root, "workload", "taskruns", "tenant-5--build-task.yaml")
					if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
						return err
					}
					return os.WriteFile(path, []byte("apiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  name: build-task\n  namespace: tenant-5\n"), 0o640)
				}
			}
		}
		manifest := model.RunManifest{RunID: "run-7", Fixture: model.FixtureIdentity{TenantNamespace: "tenant-5"}}

		_, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), manifest.RunID, manifest, sources)
		Expect(err).NotTo(HaveOccurred())
	})

	ginkgo.It("rejects empty, malformed, or mismatched TaskRun YAML snapshots", func() {
		for _, testCase := range []struct {
			name string
			data []byte
		}{
			{name: "empty", data: []byte("\n")},
			{name: "malformed", data: []byte("kind: [\n")},
			{name: "wrong kind", data: []byte("kind: PipelineRun\nmetadata:\n  name: build-task\n  namespace: tenant-5\n")},
			{name: "wrong name", data: []byte("kind: TaskRun\nmetadata:\n  name: another-task\n  namespace: tenant-5\n")},
			{name: "wrong namespace", data: []byte("kind: TaskRun\nmetadata:\n  name: build-task\n  namespace: another-tenant\n")},
		} {
			ginkgo.By(testCase.name)
			root := ginkgo.GinkgoT().TempDir()
			sources := taskRunSnapshotTestSources(testCase.data)
			manifest := model.RunManifest{RunID: "run-8", Fixture: model.FixtureIdentity{TenantNamespace: "tenant-5"}}

			_, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), manifest.RunID, manifest, sources)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workload/taskruns/tenant-5--build-task.yaml"))
		}
	})

	ginkgo.It("requires each available failed-build log path to be a saved regular file", func() {
		for _, testCase := range []struct {
			name      string
			createLog func(string) error
			wantError bool
		}{
			{name: "missing", wantError: true},
			{name: "directory", createLog: func(path string) error { return os.MkdirAll(path, 0o750) }, wantError: true},
			{name: "regular file", createLog: func(path string) error { return os.WriteFile(path, []byte("log"), 0o640) }},
		} {
			ginkgo.By(testCase.name)
			stateDir := filepath.Join(ginkgo.GinkgoT().TempDir(), "state")
			logPath := filepath.Join(stateDir, "run-6", "failed-builds", "build.log")
			sources := requiredTestSources()
			sources = append(sources, Source{Name: "failed-builds", Path: "failed-builds/", Collect: func(_ context.Context, runRoot string) error {
				if err := os.MkdirAll(filepath.Join(runRoot, "failed-builds"), 0o750); err != nil {
					return err
				}
				if testCase.createLog != nil {
					return testCase.createLog(logPath)
				}
				return nil
			}})
			manifest := model.RunManifest{
				RunID:           "run-6",
				FailedBuildLogs: []model.FailedBuildLog{{TaskRunName: "build-task", LogPath: logPath}},
			}

			_, err := (Collector{StateDir: stateDir}).CollectAndVerify(context.Background(), manifest.RunID, manifest, sources)
			if testCase.wantError {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed-builds/build.log"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	})

	ginkgo.It("accepts failed-build logs addressed from the run root or state directory", func() {
		originalWorkingDirectory, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		temporaryWorkingDirectory := ginkgo.GinkgoT().TempDir()
		Expect(os.Chdir(temporaryWorkingDirectory)).To(Succeed())
		defer func() { _ = os.Chdir(originalWorkingDirectory) }()

		for _, pathMode := range []string{"run-root-relative", "state-dir-relative"} {
			ginkgo.By(pathMode)
			stateDir := ".konflux-test-runs"
			artifactDirectory := "run-9-" + strings.ReplaceAll(pathMode, "-", "_")
			logPath := filepath.Join("failed-builds", "build.log")
			if pathMode == "state-dir-relative" {
				logPath = filepath.Join(stateDir, artifactDirectory, logPath)
			}
			sources := requiredTestSources()
			sources = append(sources, Source{Name: "failed-builds", Path: "failed-builds/", Collect: func(_ context.Context, root string) error {
				logDirectory := filepath.Join(root, "failed-builds")
				if err := os.MkdirAll(logDirectory, 0o750); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(logDirectory, "build.log"), []byte("failure log"), 0o640)
			}})
			manifest := model.RunManifest{
				RunID:             artifactDirectory,
				ArtifactDirectory: artifactDirectory,
				FailedBuildLogs:   []model.FailedBuildLog{{TaskRunName: "build-task", LogPath: logPath}},
			}

			_, err := (Collector{StateDir: stateDir}).CollectAndVerify(context.Background(), manifest.RunID, manifest, sources)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	ginkgo.It("resolves state-dir-relative logs when the state directory is named failed-builds", func() {
		originalWorkingDirectory, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		temporaryWorkingDirectory := ginkgo.GinkgoT().TempDir()
		Expect(os.Chdir(temporaryWorkingDirectory)).To(Succeed())
		defer func() { _ = os.Chdir(originalWorkingDirectory) }()

		stateDir := "failed-builds"
		artifactDirectory := "run-10-custom-state-dir"
		logPath := filepath.Join(stateDir, artifactDirectory, "failed-builds", "build.log")
		sources := requiredTestSources()
		sources = append(sources, Source{Name: "failed-builds", Path: "failed-builds/", Collect: func(_ context.Context, root string) error {
			logDirectory := filepath.Join(root, "failed-builds")
			if err := os.MkdirAll(logDirectory, 0o750); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(logDirectory, "build.log"), []byte("failure log"), 0o640)
		}})
		manifest := model.RunManifest{
			RunID:             artifactDirectory,
			ArtifactDirectory: artifactDirectory,
			FailedBuildLogs:   []model.FailedBuildLog{{TaskRunName: "build-task", LogPath: logPath}},
		}

		_, err = (Collector{StateDir: stateDir}).CollectAndVerify(context.Background(), manifest.RunID, manifest, sources)
		Expect(err).NotTo(HaveOccurred())
	})

	ginkgo.It("rejects failed-build log paths outside the run root or through symlinks", func() {
		root := filepath.Join(ginkgo.GinkgoT().TempDir(), "run")
		outside := filepath.Join(ginkgo.GinkgoT().TempDir(), "outside")
		Expect(os.MkdirAll(filepath.Join(root, "failed-builds"), 0o750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(outside, "failed-builds"), 0o750)).To(Succeed())
		outsideLog := filepath.Join(outside, "failed-builds", "build.log")
		Expect(os.WriteFile(outsideLog, []byte("outside"), 0o640)).To(Succeed())
		symlinkLog := filepath.Join(root, "failed-builds", "linked.log")
		Expect(os.Symlink(outsideLog, symlinkLog)).To(Succeed())

		for _, logPath := range []string{
			filepath.Join("..", "outside", "failed-builds", "build.log"),
			outsideLog,
			filepath.Join("failed-builds", "linked.log"),
		} {
			Expect(verifyFailedBuildLogs(root, []model.FailedBuildLog{{LogPath: logPath}})).To(HaveOccurred(), logPath)
		}
	})

	ginkgo.It("resolves explicit failed-build paths from the run root when cwd is nested", func() {
		root := filepath.Join(ginkgo.GinkgoT().TempDir(), "run")
		sessionDirectory := filepath.Join(root, "session")
		logDirectory := filepath.Join(root, "failed-builds")
		Expect(os.MkdirAll(sessionDirectory, 0o750)).To(Succeed())
		Expect(os.MkdirAll(logDirectory, 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(logDirectory, "build.log"), []byte("failure"), 0o640)).To(Succeed())
		workingDirectory, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(sessionDirectory)).To(Succeed())
		defer func() { _ = os.Chdir(workingDirectory) }()

		err = verifyFailedBuildLogs(root, []model.FailedBuildLog{{LogPath: filepath.Join("failed-builds", "build.log")}})
		Expect(err).NotTo(HaveOccurred())
	})
})

func requiredTestSources() []Source {
	paths := []string{"workload/applications.json", "workload/components.json", "workload/pipelineruns.json", "workload/taskruns.json", "workload/taskruns/", "workload/pods.json"}
	sources := make([]Source, 0, len(paths))
	for index, path := range paths {
		path := path
		index := index
		sources = append(sources, Source{Name: filepath.Base(path), Path: path, Collect: func(_ context.Context, root string) error {
			if filepath.Ext(path) == "" {
				return os.MkdirAll(filepath.Join(root, path), 0o750)
			}
			if path == "workload/taskruns.json" {
				return writeJSON(filepath.Join(root, path), map[string]any{"items": []any{}})
			}
			return writeJSON(filepath.Join(root, path), map[string]any{"index": index})
		}})
	}
	return sources
}

func taskRunSnapshotTestSources(snapshot []byte) []Source {
	sources := requiredTestSources()
	for index := range sources {
		switch sources[index].Path {
		case "workload/taskruns.json":
			sources[index].Collect = func(_ context.Context, root string) error {
				return writeJSON(filepath.Join(root, "workload", "taskruns.json"), map[string]any{
					"items": []map[string]any{{"metadata": map[string]any{"name": "build-task"}}},
				})
			}
		case "workload/taskruns/":
			sources[index].Collect = func(_ context.Context, root string) error {
				path := filepath.Join(root, "workload", "taskruns", "tenant-5--build-task.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					return err
				}
				return os.WriteFile(path, snapshot, 0o640)
			}
		}
	}
	return sources
}
