package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	CommandRun                   = "run"
	CommandCollectLogs           = "collect-logs"
	CommandCleanup               = "cleanup"
	ProviderGitHub               = "github"
	ProviderGitLab               = "gitlab"
	DefaultStateDir              = ".konflux-test-runs"
	DefaultGitHubRepo            = "https://github.com/redhat-appstudio-qe/dr_test_mathwizz"
	DefaultGitLabRepo            = "https://gitlab.com/konflux-qe/dr_test_mathwizz_gl"
	DefaultGitHubTenantNamespace = "mathwizz-test-github"
	DefaultGitLabTenantNamespace = "mathwizz-test-gitlab"
	DefaultBranch                = "main"
)

var DefaultComponentPaths = []string{
	"web-server/Dockerfile",
	"history-worker/Dockerfile",
	"frontend/Dockerfile",
}

type Credentials struct {
	GitHubToken   string
	GitHubOrg     string
	GitLabToken   string
	GitLabAPIURL  string
	GitLabGroupID string
}

type Timeouts struct {
	Preflight time.Duration
	Build     time.Duration
	Pruning   time.Duration
	Archive   time.Duration
	Collect   time.Duration
}

type Request struct {
	Command          string
	Provider         string
	RunID            string
	ResumeRunID      string
	ClusterServer    string
	StateDir         string
	TenantNamespace  string
	ApplicationName  string
	SourceRepository string
	SourceBranch     string
	ArchiveGVR       string
	ArchiveAPIURL    string
	Registry         string
	ComponentPaths   []string
	Timeouts         Timeouts
	Credentials      Credentials
}

func Defaults() Request {
	return Request{
		StateDir:         DefaultStateDir,
		SourceRepository: DefaultGitHubRepo,
		SourceBranch:     DefaultBranch,
		TenantNamespace:  DefaultGitHubTenantNamespace,
		ApplicationName:  "mathwizz",
		ComponentPaths:   append([]string(nil), DefaultComponentPaths...),
		Timeouts: Timeouts{
			Preflight: 10 * time.Minute,
			Build:     60 * time.Minute,
			Pruning:   30 * time.Minute,
			Archive:   10 * time.Minute,
			Collect:   10 * time.Minute,
		},
	}
}

func Parse(args []string) (Request, error) {
	if len(args) == 0 {
		return Request{}, errors.New("usage: konflux-test <run|collect-logs|cleanup> ...")
	}
	req := Defaults()
	req.Command = args[0]
	remaining := args[1:]

	switch req.Command {
	case CommandRun:
		if len(remaining) == 0 {
			return Request{}, errors.New("run requires provider github or gitlab")
		}
		req.Provider = remaining[0]
		if req.Provider == ProviderGitLab {
			req.SourceRepository = DefaultGitLabRepo
			req.TenantNamespace = DefaultGitLabTenantNamespace
		}
		remaining = remaining[1:]
		if req.Provider != ProviderGitHub && req.Provider != ProviderGitLab {
			return Request{}, fmt.Errorf("unsupported provider %q", req.Provider)
		}
	case CommandCollectLogs, CommandCleanup:
		if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" || strings.HasPrefix(remaining[0], "-") {
			return Request{}, fmt.Errorf("%s requires a run ID", req.Command)
		}
		req.RunID = remaining[0]
		remaining = remaining[1:]
	default:
		return Request{}, fmt.Errorf("unsupported command %q", req.Command)
	}

	fs := flag.NewFlagSet("konflux-test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&req.ClusterServer, "cluster-server", "", "expected Kubernetes API server URL")
	fs.StringVar(&req.StateDir, "state-dir", req.StateDir, "run state directory")
	fs.StringVar(&req.TenantNamespace, "tenant-namespace", req.TenantNamespace, "persistent tenant namespace")
	fs.StringVar(&req.ApplicationName, "application", req.ApplicationName, "persistent Application name")
	fs.StringVar(&req.SourceRepository, "source-repository", req.SourceRepository, "source repository")
	fs.StringVar(&req.SourceBranch, "branch", req.SourceBranch, "source branch")
	var envFile string
	fs.StringVar(&envFile, "env-file", "", "dotenv file containing provider credentials")
	fs.StringVar(&req.ArchiveGVR, "archive-gvr", "", "KubeArchive group/version/resource")
	fs.StringVar(&req.ArchiveAPIURL, "archive-api-url", "", "KubeArchive API URL")
	fs.StringVar(&req.Registry, "registry", "", "registry endpoint")
	fs.DurationVar(&req.Timeouts.Preflight, "preflight-timeout", req.Timeouts.Preflight, "preflight timeout")
	fs.DurationVar(&req.Timeouts.Build, "build-timeout", req.Timeouts.Build, "build timeout")
	fs.DurationVar(&req.Timeouts.Pruning, "pruning-timeout", req.Timeouts.Pruning, "pruning timeout")
	fs.DurationVar(&req.Timeouts.Archive, "archive-timeout", req.Timeouts.Archive, "archive timeout")
	fs.DurationVar(&req.Timeouts.Collect, "collect-timeout", req.Timeouts.Collect, "collection timeout")
	fs.StringVar(&req.ResumeRunID, "resume", "", "resume an existing run ID")
	if err := fs.Parse(remaining); err != nil {
		return Request{}, err
	}
	if fs.NArg() != 0 {
		return Request{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if req.Command == CommandRun && req.ResumeRunID != "" {
		req.RunID = req.ResumeRunID
	}
	fileValues := map[string]string{}
	if strings.TrimSpace(envFile) != "" {
		var err error
		fileValues, err = readEnvFile(envFile)
		if err != nil {
			return Request{}, err
		}
	}
	req.Credentials = credentialsFromEnv(fileValues)
	return req, req.Validate()
}

func credentialsFromEnv(fileValues map[string]string) Credentials {
	return Credentials{
		GitHubToken:   envValue("GITHUB_TOKEN", fileValues),
		GitHubOrg:     envValue("MY_GITHUB_ORG", fileValues),
		GitLabToken:   envValue("GITLAB_BOT_TOKEN", fileValues),
		GitLabAPIURL:  envValue("GITLAB_API_URL", fileValues),
		GitLabGroupID: envValue("GITLAB_GROUP_ID", fileValues),
	}
}

func envValue(key string, fileValues map[string]string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fileValues[key]
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}
	values := make(map[string]string)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("env file %q line %d must be KEY=VALUE", path, lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	return values, nil
}

func (r Request) Validate() error {
	if r.Command == "" {
		return errors.New("command is required")
	}
	if strings.TrimSpace(r.ClusterServer) == "" {
		return errors.New("--cluster-server is required")
	}
	if r.Command == CommandRun && r.Provider != ProviderGitHub && r.Provider != ProviderGitLab {
		return fmt.Errorf("unsupported provider %q", r.Provider)
	}
	if (r.Command == CommandRun || r.Command == CommandCollectLogs || r.Command == CommandCleanup) && strings.TrimSpace(r.StateDir) == "" {
		return errors.New("state directory is required")
	}
	if r.Command != CommandRun && strings.TrimSpace(r.RunID) == "" {
		return errors.New("run ID is required")
	}
	if r.Command == CommandRun && r.ResumeRunID != "" && !validRunID(r.ResumeRunID) {
		return fmt.Errorf("invalid resume run ID %q", r.ResumeRunID)
	}
	if r.Command == CommandRun && len(r.ComponentPaths) != len(DefaultComponentPaths) {
		return fmt.Errorf("exactly %d component paths are required", len(DefaultComponentPaths))
	}
	if r.TenantNamespace == "" || r.ApplicationName == "" {
		return errors.New("tenant namespace and application are required")
	}
	for _, timeout := range []struct {
		name  string
		value time.Duration
	}{
		{"preflight", r.Timeouts.Preflight}, {"build", r.Timeouts.Build}, {"pruning", r.Timeouts.Pruning}, {"archive", r.Timeouts.Archive}, {"collect", r.Timeouts.Collect},
	} {
		if timeout.value <= 0 {
			return fmt.Errorf("%s timeout must be positive", timeout.name)
		}
	}
	if r.Command == CommandRun {
		if err := validateProviderCredentials(r.Provider, r.Credentials); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderCredentials(provider string, credentials Credentials) error {
	missing := []string{}
	switch provider {
	case ProviderGitHub:
		if strings.TrimSpace(credentials.GitHubToken) == "" {
			missing = append(missing, "GITHUB_TOKEN")
		}
		if strings.TrimSpace(credentials.GitHubOrg) == "" {
			missing = append(missing, "MY_GITHUB_ORG")
		}
	case ProviderGitLab:
		if strings.TrimSpace(credentials.GitLabToken) == "" {
			missing = append(missing, "GITLAB_BOT_TOKEN")
		}
		if strings.TrimSpace(credentials.GitLabGroupID) == "" {
			missing = append(missing, "GITLAB_GROUP_ID")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s credentials missing: %s; set them in the environment or pass --env-file", provider, strings.Join(missing, ", "))
	}
	return nil
}

func validRunID(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}
