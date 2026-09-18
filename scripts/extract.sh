#!/usr/bin/env bash
set -euo pipefail

# extract.sh — dry-run extraction of heisentick Go tree + corpus into heisentick-strat.
# Usage:
#   ./scripts/extract.sh /path/to/heisentick          # dry-run (default)
#   ./scripts/extract.sh /path/to/heisentick --apply   # execute for real
#
# This script:
#   1. Copies go/ and dsl-conformance/ from heisentick into heisentick-strat.
#   2. Rewrites Go import paths from backtester/go/* to github.com/spik3r/heisentick-strat/*.
#   3. Rewrites go.mod module declaration.
#   4. Runs gofmt, go build, go test in the destination.
#   5. Prints a diff summary.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

APPLY=0
SOURCE=""

for arg in "$@"; do
  case "$arg" in
    --apply) APPLY=1 ;;
    --help|-h)
      echo "Usage: $0 <heisentick-checkout-path> [--apply]"
      exit 0
      ;;
    *)
      if [ -z "$SOURCE" ]; then
        SOURCE="$arg"
      else
        echo "Error: unexpected argument '$arg'" >&2
        exit 1
      fi
      ;;
  esac
done

if [ -z "$SOURCE" ]; then
  echo "Error: missing heisentick checkout path." >&2
  echo "Usage: $0 <heisentick-checkout-path> [--apply]" >&2
  exit 1
fi

if [ ! -d "$SOURCE/go" ]; then
  echo "Error: $SOURCE/go does not exist." >&2
  exit 1
fi

if [ ! -f "$SOURCE/go/go.mod" ]; then
  echo "Error: $SOURCE/go/go.mod does not exist." >&2
  exit 1
fi

echo "=== heisentick-strat extraction script ==="
echo "Source: $SOURCE"
echo "Destination: $REPO_ROOT"
echo "Mode: $([ "$APPLY" -eq 1 ] && echo "APPLY (real)" || echo "DRY RUN (no changes written)")"
echo ""

# ── Step 1: Copy go/ tree ──────────────────────────────────────────────

echo "Step 1: Copy go/ tree"

GO_DEST="$REPO_ROOT"
if [ "$APPLY" -eq 1 ]; then
  # Copy individual packages to repo root (flat layout)
  for pkg in dsl engine marketdata contextcols data; do
    if [ -d "$SOURCE/go/$pkg" ]; then
      echo "  Copying go/$pkg/ -> $GO_DEST/$pkg/"
      cp -R "$SOURCE/go/$pkg" "$GO_DEST/$pkg"
    fi
  done

  # Copy cmd/ tree
  mkdir -p "$GO_DEST/cmd"
  if [ -d "$SOURCE/go/cmd/btgo" ]; then
    echo "  Copying go/cmd/btgo/ -> $GO_DEST/cmd/heisentick/"
    cp -R "$SOURCE/go/cmd/btgo" "$GO_DEST/cmd/heisentick"
  fi
  if [ -d "$SOURCE/go/cmd/dslwasm" ]; then
    echo "  Copying go/cmd/dslwasm/ -> $GO_DEST/cmd/dslwasm/"
    cp -R "$SOURCE/go/cmd/dslwasm" "$GO_DEST/cmd/dslwasm"
  fi

  # Copy go.mod and go.sum
  echo "  Copying go.mod, go.sum"
  cp "$SOURCE/go/go.mod" "$GO_DEST/go.mod"
  if [ -f "$SOURCE/go/go.sum" ]; then
    cp "$SOURCE/go/go.sum" "$GO_DEST/go.sum"
  fi
else
  echo "  [dry-run] Would copy go/ packages to $GO_DEST/"
  echo "  [dry-run] Would copy go/cmd/btgo -> $GO_DEST/cmd/heisentick/"
  echo "  [dry-run] Would copy go/cmd/dslwasm -> $GO_DEST/cmd/dslwasm/"
  echo "  [dry-run] Would copy go.mod, go.sum"
fi

# ── Step 2: Copy dsl-conformance/ ──────────────────────────────────────

echo ""
echo "Step 2: Copy dsl-conformance/ -> dsl-conformance/"

if [ -d "$SOURCE/dsl-conformance" ]; then
  if [ "$APPLY" -eq 1 ]; then
    cp -R "$SOURCE/dsl-conformance" "$REPO_ROOT/dsl-conformance"
    echo "  Copied dsl-conformance/ -> dsl-conformance/"
  else
    echo "  [dry-run] Would copy dsl-conformance/ -> dsl-conformance/"
  fi
else
  echo "  WARNING: $SOURCE/dsl-conformance not found, skipping."
fi

# ── Step 3: Rewrite Go import paths ────────────────────────────────────

echo ""
echo "Step 3: Rewrite Go import paths"

REWRITE_RULES=(
  's|"backtester/go/dsl"|"github.com/spik3r/heisentick-strat/dsl"|g'
  's|"backtester/go/engine"|"github.com/spik3r/heisentick-strat/engine"|g'
  's|"backtester/go/marketdata"|"github.com/spik3r/heisentick-strat/marketdata"|g'
  's|"backtester/go/contextcols"|"github.com/spik3r/heisentick-strat/contextcols"|g'
  's|"backtester/go/data"|"github.com/spik3r/heisentick-strat/data"|g'
  's|"backtester/go/testsupport"|"github.com/spik3r/heisentick-strat/testsupport"|g'
  's|^module backtester/go|module github.com/spik3r/heisentick-strat|g'
)

GO_FILES=$(find "$REPO_ROOT/dsl" "$REPO_ROOT/engine" "$REPO_ROOT/marketdata" \
  "$REPO_ROOT/contextcols" "$REPO_ROOT/data" "$REPO_ROOT/cmd" \
  -name '*.go' 2>/dev/null || true)

MOD_COUNT=0
for f in $GO_FILES; do
  for rule in "${REWRITE_RULES[@]}"; do
    if sed -i.bak "$rule" "$f" 2>/dev/null; then
      true
    fi
  done
  MOD_COUNT=$((MOD_COUNT + 1))
done

# Rewrite go.mod
if [ -f "$REPO_ROOT/go.mod" ]; then
  for rule in "${REWRITE_RULES[@]}"; do
    sed -i.bak "$rule" "$REPO_ROOT/go.mod" 2>/dev/null || true
  done
fi

if [ "$APPLY" -eq 1 ]; then
  echo "  Rewrote import paths in $MOD_COUNT Go files + go.mod"
  # Clean up .bak files
  find "$REPO_ROOT" -name '*.go.bak' -delete 2>/dev/null || true
  find "$REPO_ROOT" -name 'go.mod.bak' -delete 2>/dev/null || true
else
  echo "  [dry-run] Would rewrite import paths in ~$MOD_COUNT Go files"
  echo "  [dry-run] Would rewrite go.mod module declaration"
fi

# ── Step 4: Validate ───────────────────────────────────────────────────

echo ""
echo "Step 4: Validate (gofmt, go build, go test)"

if [ "$APPLY" -eq 1 ]; then
  echo ""
  echo "--- gofmt ---"
  UNFORMATTED=$(gofmt -l "$REPO_ROOT" 2>/dev/null || true)
  if [ -n "$UNFORMATTED" ]; then
    echo "Files needing gofmt:"
    echo "$UNFORMATTED"
    echo ""
    echo "Running gofmt -w ..."
    gofmt -w "$REPO_ROOT" 2>/dev/null || true
    echo "Done. Files reformatted."
  else
    echo "All files already formatted."
  fi

  echo ""
  echo "--- go vet ---"
  (cd "$REPO_ROOT" && go vet ./... 2>&1) || echo "go vet failed (see above)"

  echo ""
  echo "--- go build ---"
  BUILD_OUTPUT=""
  BUILD_OK=0
  BUILD_OUTPUT=$(cd "$REPO_ROOT" && go build ./... 2>&1) && BUILD_OK=1 || true
  if [ "$BUILD_OK" -eq 1 ]; then
    echo "go build ./... PASSED"
  else
    echo "go build ./... FAILED:"
    echo "$BUILD_OUTPUT"
  fi

  echo ""
  echo "--- go test ---"
  TEST_OUTPUT=""
  TEST_OK=0
  TEST_OUTPUT=$(cd "$REPO_ROOT" && go test ./... 2>&1) && TEST_OK=1 || true
  if [ "$TEST_OK" -eq 1 ]; then
    echo "go test ./... PASSED"
  else
    echo "go test ./... FAILED:"
    echo "$TEST_OUTPUT"
  fi

  echo ""
  echo "--- Diff summary ---"
  cd "$REPO_ROOT"
  echo "Files added:"
  git diff --stat --cached 2>/dev/null || echo "(not a git repo or no staged changes)"
  echo ""
  echo "Files changed:"
  git diff --stat 2>/dev/null || echo "(not a git repo or no unstaged changes)"
else
  echo "  [dry-run] Would run: gofmt -l, go vet ./..., go build ./..., go test ./..."
  echo "  [dry-run] No validation performed (use --apply to execute)"
fi

echo ""
echo "=== Extraction $([ "$APPLY" -eq 1 ] && echo "COMPLETE" || echo "DRY RUN COMPLETE") ==="
