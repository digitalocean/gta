#!/bin/bash

go run ./cmd/gta/main.go \
  --base "${INPUT_BASE:-}" \
  --include "${INPUT_INCLUDE:-}" \
  --merge "${INPUT_MERGE:-}" \
  --json "${INPUT_JSON:-}" \
  --buildable-only "${INPUT_BUILDABLE_ONLY:-}" \
  --changed-files "${INPUT_CHANGED_FILES:-}" \
  --tags "${INPUT_TAGS:-}"
