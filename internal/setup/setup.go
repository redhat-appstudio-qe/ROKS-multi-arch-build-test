package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultEnvFile = ".konflux-test.env"

// Prepare runs the local setup helper and adds the validated cluster settings
// required by the main CLI parser. The helper is intentionally separate from
// the workflow binary so it cannot recursively invoke this process.
func Prepare(args []string) ([]string, error) {
	if len(args) == 0 || !needsSetup(args[0]) {
		return args, nil
	}

	envFile := envFileArg(args)
	provider := ""
	if args[0] == "run" && len(args) > 1 {
		provider = args[1]
	}

	script, err := findScript()
	if err != nil {
		return nil, err
	}
	commandArgs := []string{"--setup", envFile}
	if provider != "" {
		commandArgs = append(commandArgs, provider)
	}
	command := exec.Command(script, commandArgs...)
	command.Stderr = os.Stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run local setup: %w", err)
	}
	values, err := parseOutput(output)
	if err != nil {
		return nil, err
	}

	kubeconfig := values["KUBECONFIG"]
	clusterServer := values["CLUSTER_SERVER"]
	resolvedEnvFile := values["ENV_FILE"]
	if kubeconfig == "" || clusterServer == "" || resolvedEnvFile == "" {
		return nil, fmt.Errorf("local setup returned incomplete configuration")
	}
	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		return nil, fmt.Errorf("set KUBECONFIG: %w", err)
	}

	if expected, present := flagValue(args, "--cluster-server"); present && expected != clusterServer {
		return nil, fmt.Errorf("cluster server mismatch: requested %s, active %s", expected, clusterServer)
	}
	prepared := append([]string(nil), args...)
	if _, present := flagValue(prepared, "--cluster-server"); !present {
		prepared = append(prepared, "--cluster-server", clusterServer)
	}
	if _, present := flagValue(prepared, "--env-file"); !present {
		prepared = append(prepared, "--env-file", resolvedEnvFile)
	}
	return prepared, nil
}

func needsSetup(command string) bool {
	switch command {
	case "run", "cleanup":
		return true
	default:
		return false
	}
}

func envFileArg(args []string) string {
	for index, arg := range args {
		if arg == "--env-file" && index+1 < len(args) {
			return args[index+1]
		}
		if strings.HasPrefix(arg, "--env-file=") {
			return strings.TrimPrefix(arg, "--env-file=")
		}
	}
	return defaultEnvFile
}

func flagValue(args []string, name string) (string, bool) {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return args[index+1], true
		}
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"="), true
		}
	}
	return "", false
}

func findScript() (string, error) {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("KONFLUX_TEST_SETUP_SCRIPT")); configured != "" {
		candidates = append(candidates, configured)
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(workingDirectory, "scripts", "run-local.sh"))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "..", "scripts", "run-local.sh"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("local setup script not found; expected scripts/run-local.sh")
}

func parseOutput(output []byte) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || value == "" {
			return nil, fmt.Errorf("invalid local setup output line %q", line)
		}
		switch key {
		case "KUBECONFIG", "CLUSTER_SERVER", "ENV_FILE":
			values[key] = value
		default:
			return nil, fmt.Errorf("unexpected local setup output key %q", key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read local setup output: %w", err)
	}
	return values, nil
}
