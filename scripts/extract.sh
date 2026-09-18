#!/usr/bin/env bash
# extract.sh — T-B2 mechanical extraction of the Strat language and Go engine
# from heisentick into heisentick-strat.
#
# Usage:
#   scripts/extract.sh <heisentick-clone>                        dry run: verify the
#                                                                 source and print the plan
#   scripts/extract.sh --apply <heisentick-clone> <dest> [opts]  copy, rewrite, validate
#
# Options (apply mode only):
#   --fix-paths   also apply the fixture-path fixes listed in docs/extraction-plan.md
#                 (the pure rewrite alone does not pass `go test ./...`)
#   --skip-tests  stop after `go build ./...`
#
# The script never writes to <heisentick-clone>. Run it against a fresh clone
# of https://github.com/spik3r/heisentick, not a working checkout, so the
# source is exactly one commit. <dest> is the heisentick-strat checkout (or a
# scratch directory for a rehearsal); the script refuses to overwrite any
# target path that already exists there.
#
# Mapping and rewrite rules: docs/layout.md. Observed results: docs/extraction-plan.md.

set -euo pipefail

MODULE="github.com/spik3r/heisentick-strat"

# source dir -> destination dir (relative to each root). `go/dsl` and
# `go/engine` are import-path facades and are not copied; see docs/layout.md.
MAPPING=(
  "go/native:engine"
  "strat/implementations/server-runtime:dsl"
  "go/marketdata:marketdata"
  "go/contextcols:contextcols"
  "go/data:data"
  "go/testsupport:testsupport"
  "go/cmd/heisentick:cmd/heisentick"
  "go/cmd/dslwasm:cmd/dslwasm"
  "go/cmd/enginewasm:cmd/enginewasm"
  "strat/conformance:conformance"
  "strat/specification:spec"
  "strat/docs:spec"
  "strat/examples:examples"
)

# Files copied individually (the rest of their directory is dropped).
SINGLE_FILES=(
  "go/dsl/facade_contract_test.go:dsl/facade_contract_test.go"
)

# Go import rewrites: old path -> new path (both without quotes).
IMPORT_RULES=(
  "heisentick/go/native:$MODULE/engine"
  "heisentick/go/engine:$MODULE/engine"
  "heisentick/go/dsl:$MODULE/dsl"
  "heisentick/strat/implementations/server-runtime:$MODULE/dsl"
  "heisentick/go/marketdata:$MODULE/marketdata"
  "heisentick/go/contextcols:$MODULE/contextcols"
  "heisentick/go/data:$MODULE/data"
  "heisentick/go/testsupport:$MODULE/testsupport"
)

usage() {
  sed -n '2,21p' "$0" | sed 's/^# \{0,1\}//'
}

die() {
  echo "error: $*" >&2
  exit 1
}

verify_source() {
  local src="$1"
  [ -f "$src/go.mod" ] || die "$src/go.mod not found"
  grep -q '^module heisentick$' "$src/go.mod" || die "$src/go.mod does not declare 'module heisentick' (stale checkout?)"
  local entry from
  for entry in "${MAPPING[@]}"; do
    from="${entry%%:*}"
    [ -d "$src/$from" ] || die "$src/$from not found"
  done
  for entry in "${SINGLE_FILES[@]}"; do
    from="${entry%%:*}"
    [ -f "$src/$from" ] || die "$src/$from not found"
  done
  [ -d "$src/go/dsl" ] || die "$src/go/dsl not found"
  [ -d "$src/go/engine" ] || die "$src/go/engine not found"
  if [ -d "$src/.git" ]; then
    echo "source commit: $(git -C "$src" log -1 --format='%h %ad %s' --date=short)"
  fi
}

count_files() {
  find "$1" -type f | wc -l | tr -d ' '
}

print_plan() {
  local src="$1"
  echo
  printf '%-45s %-22s %6s\n' "source" "destination" "files"
  local entry from to
  for entry in "${MAPPING[@]}"; do
    from="${entry%%:*}"
    to="${entry##*:}"
    printf '%-45s %-22s %6s\n' "$from/" "$to/" "$(count_files "$src/$from")"
  done
  for entry in "${SINGLE_FILES[@]}"; do
    from="${entry%%:*}"
    to="${entry##*:}"
    printf '%-45s %-22s %6s\n' "$from" "$to" 1
  done
  printf '%-45s %-22s %6s\n' "go/dsl/ (facade, rest)" "dropped" "$(( $(count_files "$src/go/dsl") - 1 ))"
  printf '%-45s %-22s %6s\n' "go/engine/ (facade)" "dropped" "$(count_files "$src/go/engine")"
  printf '%-45s %-22s %6s\n' "go.mod" "go.mod (module $MODULE)" 1
  echo
  echo "import lines in the source (facade files included):"
  local rule old
  for rule in "${IMPORT_RULES[@]}"; do
    old="${rule%%:*}"
    printf '  %-55s %s\n' "\"$old\"" "$(grep -rF --include='*.go' "\"$old\"" "$src/go" "$src/strat" | wc -l | tr -d ' ' || true)"
  done
}

copy_tree() {
  local src="$1" dest="$2"
  local entry from to
  for entry in "${MAPPING[@]}"; do
    to="${entry##*:}"
    # strat/specification and strat/docs both land in spec/; allow the second.
    if [ -e "$dest/$to" ] && [ "$to" != "spec" ]; then
      die "$dest/$to already exists; refusing to overwrite"
    fi
  done
  [ -e "$dest/go.mod" ] && die "$dest/go.mod already exists; refusing to overwrite"
  mkdir -p "$dest"
  for entry in "${MAPPING[@]}"; do
    from="${entry%%:*}"
    to="${entry##*:}"
    mkdir -p "$dest/$to"
    cp -R "$src/$from/." "$dest/$to/"
  done
  for entry in "${SINGLE_FILES[@]}"; do
    from="${entry%%:*}"
    to="${entry##*:}"
    mkdir -p "$(dirname "$dest/$to")"
    cp "$src/$from" "$dest/$to"
  done
  local goline
  goline="$(grep -E '^go [0-9.]+$' "$src/go.mod")"
  printf 'module %s\n\n%s\n' "$MODULE" "$goline" > "$dest/go.mod"
  [ -f "$src/go.sum" ] && cp "$src/go.sum" "$dest/go.sum"
  return 0
}

rewrite_imports() {
  local dest="$1"
  local rule old new n
  for rule in "${IMPORT_RULES[@]}"; do
    old="${rule%%:*}"
    new="${rule#*:}"
    n="$(grep -rlF --include='*.go' "\"$old\"" "$dest" | wc -l | tr -d ' ' || true)"
    if [ "$n" != "0" ]; then
      grep -rlF --include='*.go' "\"$old\"" "$dest" | xargs perl -pi -e "s{\"\Q$old\E\"}{\"$new\"}g"
    fi
    printf '  %-75s %s files\n' "\"$old\" -> \"$new\"" "$n"
  done
  # The parser package is named after its new directory.
  n="$(grep -lE '^package serverruntime$' "$dest"/dsl/*.go | wc -l | tr -d ' ' || true)"
  perl -pi -e 's{^package serverruntime$}{package dsl}' "$dest"/dsl/*.go
  printf '  %-75s %s files\n' "package serverruntime -> package dsl" "$n"
  local left
  left="$(grep -rnE --include='*.go' '"heisentick/' "$dest" || true)"
  if [ -n "$left" ]; then
    echo "unrewritten imports remain:" >&2
    echo "$left" >&2
    exit 1
  fi
}

# Fixture-path fixes. Each is a one-line change recorded in
# docs/extraction-plan.md; none changes engine or parser behaviour.
fix_paths() {
  local src="$1" dest="$2"
  echo "  testsupport/reporoot.go: repo marker go.mod only; corpus at conformance/"
  perl -pi -e 's{isFile\(filepath\.Join\(dir, "package\.json"\)\) && isFile\(filepath\.Join\(dir, "go\.mod"\)\)}{isFile(filepath.Join(dir, "go.mod"))}; s{filepath\.Join\(MustRepoRoot\(\), "strat", "conformance"\)}{filepath.Join(MustRepoRoot(), "conformance")}' "$dest/testsupport/reporoot.go"
  echo "  testsupport/reporoot_test.go: root is one level up; alternate cwd is cmd/"
  perl -pi -e 's{filepath\.Join\(filepath\.Dir\(source\), "\.\.", "\.\."\)}{filepath.Join(filepath.Dir(source), "..")}; s{filepath\.Join\(wantRoot, "go"\)}{filepath.Join(wantRoot, "cmd")}; s{filepath\.Join\(wantRoot, "strat", "conformance"\)}{filepath.Join(wantRoot, "conformance")}g' "$dest/testsupport/reporoot_test.go"
  echo "  cmd/enginewasm/bridge_test.go: corpus glob two levels up"
  perl -pi -e 's{"\.\./\.\./\.\./strat/conformance/run/}{"../../conformance/run/}' "$dest/cmd/enginewasm/bridge_test.go"
  echo "  dsl/contract_schemas_test.go: schemas under spec/"
  perl -pi -e 's{filepath\.Join\(testsupport\.MustRepoRoot\(\), "strat", "specification", "schemas"\)}{filepath.Join(testsupport.MustRepoRoot(), "spec", "schemas")}' "$dest/dsl/contract_schemas_test.go"
  echo "  engine/testdata/strategies: frozen copies of two archived .strat sources"
  mkdir -p "$dest/engine/testdata/strategies"
  cp "$src/strategies/source/dslRoundNumberConfluence.strat" "$src/strategies/source/dslFailedBreakoutLimitEntry.strat" "$dest/engine/testdata/strategies/"
  perl -pi -e 's{filepath\.Join\("\.\.", "\.\.", "strategies", "source", }{filepath.Join("testdata", "strategies", }' "$dest/engine/sweep_rule_grade_test.go" "$dest/engine/limit_entry_test.go"
}

validate() {
  local dest="$1" skip_tests="$2"
  local status=0
  echo
  echo "== gofmt -l"
  local unformatted
  unformatted="$(cd "$dest" && gofmt -l .)"
  if [ -n "$unformatted" ]; then
    echo "$unformatted"
    echo "(running gofmt -w on the files above)"
    (cd "$dest" && echo "$unformatted" | xargs gofmt -w)
  else
    echo "clean"
  fi
  echo
  echo "== go vet ./..."
  if (cd "$dest" && go vet ./...); then echo "ok"; else status=1; fi
  echo
  echo "== go build ./..."
  if (cd "$dest" && go build ./...); then echo "ok"; else status=1; fi
  if [ "$skip_tests" = "1" ]; then
    return "$status"
  fi
  echo
  echo "== go test ./..."
  local log
  log="$(mktemp -t extract-go-test)"
  if (cd "$dest" && go test ./... > "$log" 2>&1); then
    echo "ok"
  else
    status=1
    echo "failures:"
    grep -E '^(FAIL|--- FAIL|panic:)' "$log" | sed 's/^/  /' || true
    echo "full log: $log"
  fi
  grep -E '^(ok|FAIL|\?)' "$log" | sed 's/^/  /'
  return "$status"
}

diff_summary() {
  local dest="$1"
  echo
  echo "== destination summary"
  local entry to seen=""
  for entry in "${MAPPING[@]}"; do
    to="${entry##*:}"
    case " $seen " in *" $to "*) continue ;; esac
    seen="$seen $to"
    printf '  %-22s %6s files %8s KB\n' "$to/" "$(count_files "$dest/$to")" "$(du -sk "$dest/$to" | cut -f1)"
  done
  printf '  %-22s %6s Go files\n' "total" "$(find "$dest" -name '*.go' | wc -l | tr -d ' ')"
  if git -C "$dest" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    printf '  %-22s %6s paths\n' "git status --short" "$(git -C "$dest" status --short | wc -l | tr -d ' ')"
  fi
}

main() {
  local apply=0 fix=0 skip_tests=0 src="" dest=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --apply) apply=1 ;;
      --fix-paths) fix=1 ;;
      --skip-tests) skip_tests=1 ;;
      -h|--help) usage; exit 0 ;;
      -*) die "unknown option $1" ;;
      *)
        if [ -z "$src" ]; then src="$1"
        elif [ -z "$dest" ]; then dest="$1"
        else die "unexpected argument $1"; fi
        ;;
    esac
    shift
  done
  [ -n "$src" ] || { usage; exit 1; }
  src="$(cd "$src" && pwd)"

  echo "== source: $src"
  verify_source "$src"

  if [ "$apply" = "0" ]; then
    echo "mode: dry run (nothing written)"
    print_plan "$src"
    echo
    echo "run with --apply <clone> <dest> to copy, rewrite and validate."
    return 0
  fi

  [ -n "$dest" ] || die "--apply needs <dest>"
  mkdir -p "$dest"
  dest="$(cd "$dest" && pwd)"
  case "$dest" in
    "$src"|"$src"/*) die "dest must not be inside the source clone" ;;
  esac
  echo "== dest: $dest"
  echo "mode: apply$([ "$fix" = "1" ] && echo ' + fix-paths')"

  echo
  echo "== copy"
  copy_tree "$src" "$dest"
  print_plan "$src" | sed -n '2,$p'

  echo
  echo "== rewrite imports"
  rewrite_imports "$dest"

  if [ "$fix" = "1" ]; then
    echo
    echo "== fix paths"
    fix_paths "$src" "$dest"
  fi

  local status=0
  validate "$dest" "$skip_tests" || status=$?
  diff_summary "$dest"
  echo
  if [ "$status" = "0" ]; then
    echo "== result: all checks passed"
  else
    echo "== result: checks failed (see above)"
  fi
  return "$status"
}

main "$@"
