#!/usr/bin/env bash

set -Eeuo pipefail

readonly STACK_NAME="mod05-sandbox"
readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

cd "$SCRIPT_DIR"

for command in sam aws python3; do
    if ! command -v "$command" >/dev/null 2>&1; then
        printf 'Error: required command "%s" was not found in PATH.\n' "$command" >&2
        exit 1
    fi
done

if ! python3 -c 'import boto3' >/dev/null 2>&1; then
    printf '%s\n' \
        'Error: Python package "boto3" is required by tests/test_stack.py.' \
        'On CachyOS/Arch, install it with: sudo pacman -S python-boto3' >&2
    exit 1
fi

printf 'Building...\n'
sam build --template-file template.yaml

printf "Deploying stack '%s'...\n" "$STACK_NAME"
sam deploy \
    --stack-name "$STACK_NAME" \
    --resolve-s3 \
    --capabilities CAPABILITY_IAM \
    --no-confirm-changeset \
    --no-fail-on-empty-changeset

printf '\nSeeding one Performance item (Id = perf-001) so the live demo has something to validate against...\n'

seed_item_path="$(mktemp "${TMPDIR:-/tmp}/mod05-seed-item.XXXXXX.json")"
trap 'rm -f -- "$seed_item_path"' EXIT
printf '%s' '{"Id":{"S":"perf-001"},"title":{"S":"Fall Showcase"}}' > "$seed_item_path"

if ! aws dynamodb put-item \
    --table-name mod05_Performances \
    --item "file://$seed_item_path"; then
    printf 'Seeding perf-001 failed -- smoke tests below will fail too. Fix seeding before re-running.\n' >&2
    exit 1
fi

printf '\nStack outputs:\n'
aws cloudformation describe-stacks \
    --stack-name "$STACK_NAME" \
    --query 'Stacks[0].Outputs' \
    --output table

printf '\nRunning post-deploy smoke tests (sign_up_for_audition, notify_director)...\n'
if ! python3 tests/test_stack.py; then
    printf '\nOne or more Lambdas failed smoke testing -- see output above. Stack is deployed but NOT verified.\n' >&2
    exit 1
fi

printf '\nDone. mod05-sign-up-for-audition and mod05-notify-director-consumer are live, wired together, and verified.\n'
