# Local Multi-Architecture Build Testing Tool for ROKS Konflux Clusters

`konflux-test` validates multi-architecture build creation on `kflux-lw-p01`.
It reuses fixed repositories, appends one harmless comment to each of their
three Dockerfiles, waits for matching successful PipelineRuns, verifies
pre-publication `linux/amd64` and `linux/arm64` build outputs, then stops.

Fixed fixtures:

- [dr_test_mathwizz](https://github.com/redhat-appstudio-qe/dr_test_mathwizz)
- [dr_test_mathwizz_gl](https://gitlab.com/konflux-qe/dr_test_mathwizz_gl)

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
  `.tekton` configuration available on each fixture's default branch. The
  harness waits for PaC readiness before writing trigger comments and does not
  merge onboarding changes.

## Commands

```bash
cd konflux-test
./bin/konflux-test run github --env-file .konflux-test.env
./bin/konflux-test run gitlab --env-file .konflux-test.env
./bin/konflux-test run both --env-file .konflux-test.env
./bin/konflux-test cleanup --run-id <run-id> --env-file .konflux-test.env
```

`run both` starts the fixed GitHub and GitLab fixture runs concurrently. Each
uses its own persistent tenant namespace and run ID. `cleanup` loads the run
manifest to select its recorded namespace and cluster, verifies both exact
ownership labels, and asks for confirmation before deleting the namespace.

Each run creates one tenant namespace labeled with
`app.konflux-ci.org/managed-by=konflux-test` and
`app.konflux.org/run-id=<run-id>`.

If the configured namespace already exists, an owned stale namespace requires
approval before deletion. An unowned namespace stops the run. A resumed run
may reuse only a namespace with both exact labels and the resumed run ID.

After success, the harness prompts before deleting the current owned tenant
namespace. A declined prompt retains the namespace and successful result.
After failure, it saves and verifies required artifacts before prompting.

## Environment configuration

Copy `.konflux-test.env.example` to `.konflux-test.env`. Keep the copy local.
Set `GITHUB_TOKEN` for GitHub runs and `GITLAB_BOT_TOKEN` for GitLab runs. Both
tokens are required for `run both`. Set `GITLAB_API_URL` when needed. Set
`KUBECONFIG` and the explicit `KONFLUX_CLUSTER_SERVER` guard in the same file.

The parser reads `KEY=VALUE` lines. It does not execute the environment file.

## Artifact retention

Runtime evidence is stored under `.konflux-test-runs/`.

- Each run directory includes its provider, start time, and run ID, for example
  `github-run-<hh:mm_d.m.y>_<run-id>`.
- `latest/` resolves to the most recently started run directory from run
  creation. It is a relative symlink, exposes artifacts as they are written,
  and continues to point to that run through success or failure. The next run
  atomically changes the pointer; prior run-specific directories remain
  intact. If an older directory-form `latest` cannot be atomically exchanged
  on the current filesystem, the tool preserves it and refuses to switch.
- Each run stores `manifest.json` and `status.json`.
- Failed runs store `session/manifest.json`, workload snapshots for
  Applications, Components, PipelineRuns, TaskRuns, and Pods, plus
  `collection-report.json`.
- Available pod logs are stored under
  `logs/<namespace>/<pod>/<container>.log`. Log requests use the run start
  time as `SinceTime` and stop with the run's collection context; historical
  logs are not requested.
- Credential-like fields in JSON evidence are redacted before writing.

Incomplete failure-artifact collection suppresses cleanup prompting and retains
the namespace.
