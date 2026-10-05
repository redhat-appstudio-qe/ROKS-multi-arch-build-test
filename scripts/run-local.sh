#!/usr/bin/env bash

set -euo pipefail

usage() {
    printf 'Usage: %s --setup [env-file] [github|gitlab]\n' "$(basename "$0")" >&2
}

trim() {
    local value=$1
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    printf '%s' "$value"
}

env_value() {
    local requested_key=$1
    local line key value result found=1

    while IFS= read -r line || [[ -n "$line" ]]; do
        line=$(trim "$line")
        [[ -z "$line" || "${line:0:1}" == '#' ]] && continue

        if [[ "$line" == export[[:space:]]* ]]; then
            line=$(trim "${line#export}")
        fi
        [[ "$line" == *=* ]] || continue

        key=$(trim "${line%%=*}")
        [[ "$key" == "$requested_key" ]] || continue

        value=$(trim "${line#*=}")
        if [[ ${#value} -ge 2 && "${value:0:1}" == "'" && "${value: -1}" == "'" ]]; then
            value=${value:1:${#value}-2}
        elif [[ ${#value} -ge 2 && "${value:0:1}" == '"' && "${value: -1}" == '"' ]]; then
            value=${value:1:${#value}-2}
        fi
        result=$value
        found=0
    done <"$ENV_FILE"

    if (( found == 0 )); then
        printf '%s' "$result"
    fi
    return "$found"
}

resolve_path() {
    local path=$1
    local base_dir=$2

    if [[ "$path" == /* ]]; then
        printf '%s' "$path"
    else
        printf '%s/%s' "$base_dir" "$path"
    fi
}

if [[ "${1:-}" == "--setup" ]]; then
    shift
else
    printf '%s\n' 'The setup helper must be called with --setup.' >&2
    usage
    exit 2
fi

env_file_input=${1:-.konflux-test.env}
provider=${2:-}

if [[ $# -gt 2 ]]; then
    usage
    exit 2
fi

case "$provider" in
    github|gitlab|'') ;;
    *)
        printf 'Unsupported provider: %s\n' "$provider" >&2
        usage
        exit 2
        ;;
esac

if [[ "$env_file_input" == /* ]]; then
    ENV_FILE=$env_file_input
else
    ENV_FILE=$PWD/$env_file_input
fi

if [[ ! -r "$ENV_FILE" ]]; then
    printf 'Env file is not readable: %s\n' "$ENV_FILE" >&2
    exit 1
fi

env_dir=$(cd -- "$(dirname -- "$ENV_FILE")" && pwd)
ENV_FILE=$env_dir/$(basename -- "$ENV_FILE")

kubeconfig_value=${KUBECONFIG:-}
if [[ -z "$kubeconfig_value" ]]; then
    kubeconfig_value=$(env_value KUBECONFIG || true)
fi
if [[ -z "$kubeconfig_value" ]]; then
    printf 'KUBECONFIG is required in the environment or env file: %s\n' "$ENV_FILE" >&2
    exit 1
fi
if [[ "$kubeconfig_value" == *:* ]]; then
    printf 'KUBECONFIG must name one kubeconfig file for this setup helper\n' >&2
    exit 1
fi

KUBECONFIG=$(resolve_path "$kubeconfig_value" "$env_dir")
if [[ ! -r "$KUBECONFIG" ]]; then
    printf 'KUBECONFIG is not readable: %s\n' "$KUBECONFIG" >&2
    exit 1
fi
export KUBECONFIG

oc_command=${KONFLUX_TEST_OC:-oc}
cluster_server=$("$oc_command" whoami --show-server) || {
    printf 'Unable to determine cluster server with %s whoami --show-server\n' "$oc_command" >&2
    exit 1
}
if [[ -z "$cluster_server" ]]; then
    printf 'oc returned an empty cluster server\n' >&2
    exit 1
fi

expected_server=${KONFLUX_CLUSTER_SERVER:-}
if [[ -z "$expected_server" ]]; then
    expected_server=$(env_value KONFLUX_CLUSTER_SERVER || true)
fi
if [[ -z "$expected_server" ]]; then
    printf 'KONFLUX_CLUSTER_SERVER is required for the kflux-lw-p01 guard: %s\n' "$ENV_FILE" >&2
    exit 1
fi
if [[ "$expected_server" != "$cluster_server" ]]; then
    printf 'Cluster server mismatch: expected %s, active %s\n' "$expected_server" "$cluster_server" >&2
    exit 1
fi

printf 'KUBECONFIG=%s\n' "$KUBECONFIG"
printf 'CLUSTER_SERVER=%s\n' "$cluster_server"
printf 'ENV_FILE=%s\n' "$ENV_FILE"
