#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org GOPRIVATE= GONOSUMDB= GONOPROXY=
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export npm_config_registry=https://registry.npmjs.org/

prepare_ts() { (cd core/ts && npm ci --ignore-scripts && npm run build); }
prepare_bins() {
  cargo build --locked --release --bins
  mkdir -p conformance/.bin
  (cd core/go && go build -mod=readonly -o ../../conformance/.bin/horos-conform-go ./cmd/horos-conform)
  prepare_ts
}
case "${1:-all}" in
  go)
    cd core/go
    format_status=0
    test -z "$(gofmt -l .)" || format_status=1
    go vet -mod=readonly ./...
    go test -mod=readonly -race -count=1 ./...
    test "$format_status" -eq 0
    ;;
  rust)
    cargo fmt --all -- --check
    cargo clippy --locked --workspace --all-targets -- -D warnings
    cargo test --locked --workspace
    ;;
  typescript)
    prepare_ts
    (cd core/ts && npm run lint && npm test)
    python3 scripts/consumer.py typescript
    ;;
  python)
    : "${ONTOS_PY_ROOT:?Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout}"
    python3 -m unittest discover core/py/test -v
    ;;
  conformance)
    prepare_bins
    python3 conformance/run.py
    python3 conformance/run.py --seed 1 --count "${HOROS_FUZZ_COUNT:-2000}"
    python3 conformance/ground.py
    ;;
  public)
    python3 scripts/verify-public.py
    python3 -m unittest discover scripts -p 'test_*.py' -v
    node --test scripts/test-provenance.cjs scripts/test-diagnose-npm-oidc.cjs
    git diff --check
    ;;
  all)
    for target in public go rust typescript python conformance; do bash scripts/check.sh "$target"; done
    ;;
  *) echo "unknown check target: $1" >&2; exit 2 ;;
esac
