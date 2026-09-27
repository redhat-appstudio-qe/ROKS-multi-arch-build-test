# Local Konflux Build Validation

`konflux-test` is a local operator harness for validating an existing Konflux
cluster. It does not provision Konflux, install release infrastructure, delete
provider forks or registry images, or strip finalizers.

## Prerequisites

- An existing kubeconfig context and an explicit expected API server URL.
- Read access to the Konflux APIs, KubeArchive, controller logs, and registry
  metadata; write access only for the persistent test fixture and harmless
  Dockerfile trigger commits.
- `GITHUB_TOKEN` and `MY_GITHUB_ORG` for GitHub, or `GITLAB_BOT_TOKEN`,
  `GITLAB_API_URL`, and `GITLAB_GROUP_ID` for GitLab. Store them in the ignored
  `.konflux-test.env` file when running from `konflux-test/`.
- Both linux/amd64 and linux/arm64 build capacity.

The GitLab fixture is `https://gitlab.com/konflux-qe/dr_test_mathwizz_gl`.
The default GitHub fixture source is
`https://github.com/redhat-appstudio-qe/dr_test_mathwizz`.

The GitHub default tenant namespace is `mathwizz-test-github`. The GitLab
default tenant namespace is `mathwizz-test-gitlab`. Existing fixture drift fails
closed; the harness does not silently rewrite it. Runtime state is written
under `.konflux-test-runs/<run-id>/` and credentials are redacted before
evidence is written.

## Commands

```bash
cd konflux-test
oc whoami --show-server
go run ./cmd/konflux-test run github --cluster-server "$KONFLUX_CLUSTER_SERVER" --env-file .konflux-test.env
go run ./cmd/konflux-test run gitlab --cluster-server "$KONFLUX_CLUSTER_SERVER" --env-file .konflux-test.env
go run ./cmd/konflux-test collect-logs <run-id> --cluster-server "$KONFLUX_CLUSTER_SERVER"
go run ./cmd/konflux-test cleanup <run-id> --cluster-server "$KONFLUX_CLUSTER_SERVER"
```

The only trigger-side external mutation is the timestamped Dockerfile comment
commit in the persistent provider fork. Failed runs remain available for
`collect-logs`; cleanup is explicit and ownership-checked.
