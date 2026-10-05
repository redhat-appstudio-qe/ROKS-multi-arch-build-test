# Local Konflux Build Validation

`konflux-test` validates multi-architecture build creation on `kflux-lw-p01`.
It reuses fixed repositories, appends one harmless comment to each of their
three Dockerfiles, waits for matching successful PipelineRuns, verifies
pre-publication `linux/amd64` and `linux/arm64` build outputs, then stops.

Fixed fixtures:

- GitHub: `https://github.com/redhat-appstudio-qe/dr_test_mathwizz`
- GitLab: `https://gitlab.com/konflux-qe/dr_test_mathwizz_gl`

The harness never copies, creates, forks, renames, or deletes repositories. It
does not query registries, archives, pruning, backup services, or unrelated
resources.

## Prerequisites

- An existing kubeconfig context for `kflux-lw-p01`.
- An explicit expected API server. The setup helper verifies it with
  `oc whoami --show-server`.
- A ready `multi-platform-controller` deployment and `host-config` entries for
  `linux/amd64` and `linux/arm64`. Dynamic arm64 provisioning is sufficient;
  physical arm64 nodes are not required.
- PaC onboarding must be complete for all three components, with current
  `.tekton` configuration available on the fixture's default branch. The
  harness waits for PaC readiness before writing trigger comments and does not
  merge onboarding changes.
- `GITHUB_TOKEN` for GitHub runs, or `GITLAB_BOT_TOKEN` and optional
  `GITLAB_API_URL` for GitLab runs.

Copy `.konflux-test.env.example` to `.konflux-test.env`. Keep the copy local.

## Commands

```bash
cd konflux-test
./bin/konflux-test run github --env-file .konflux-test.env
./bin/konflux-test run gitlab --env-file .konflux-test.env
```

Each run creates one tenant namespace labeled with
`app.konflux-ci.org/managed-by=konflux-test` and
`app.konflux.org/run-id=<run-id>`.

If the configured namespace already exists, an owned stale namespace requires
approval before deletion. An unowned namespace stops the run. A resumed run
may reuse only a namespace with both exact labels and the resumed run ID.

After success, the harness prompts before deleting the current owned tenant
namespace. A declined prompt retains the namespace and successful result.
After failure, it saves and verifies these artifacts before prompting:

```text
session/manifest.json
workload/applications.json
workload/components.json
workload/pipelineruns.json
workload/taskruns.json
workload/pods.json
collection-report.json
```

Incomplete artifact collection suppresses cleanup prompting and retains the
namespace. Runtime state is stored under `.konflux-test-runs/<run-id>/` with
redaction applied before evidence is written.
