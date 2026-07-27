#!/usr/bin/env bash

# Deletes the entire module05 sandbox stack -- both tables, the queue,
# both Lambdas (sign_up_for_audition, notify_director), the SQS trigger,
# and every IAM role/policy SAM created for them. One command, nothing
# left behind. Run this after each session.

set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

cd "$SCRIPT_DIR"

if ! command -v sam >/dev/null 2>&1; then
    printf 'Error: required command "sam" was not found in PATH.\n' >&2
    exit 1
fi

sam delete --stack-name mod05-sandbox --no-prompts
