# Releasing

This repo publishes three independently versioned Go modules from one
`go.work` workspace: the core `github.com/tafaquh/aerr` (tags `vX.Y.Z`)
and the adapters `github.com/tafaquh/aerr/zerolog` and
`github.com/tafaquh/aerr/zap` (tags `zerolog/vX.Y.Z`, `zap/vX.Y.Z`).

## Why the core goes first

Inside the workspace every module builds against the checked-out core,
but consumers resolve each adapter's `require github.com/tafaquh/aerr
vX.Y.Z` from the module proxy. An adapter that starts using core API that
is not yet tagged builds here and breaks for everyone else. CI's
`consumer-smoke` job (the adapters built with `GOWORK=off`) catches
exactly that, and it is why the zerolog `v1.0.0` tag is retracted.

So: tag the core, point the adapters at it, then tag the adapters. A
release that changes only an adapter starts at step 3.

## Preflight

- [ ] `main` is green on every CI job: `test` (both Go versions),
      `consumer-smoke`, `lint`, and `fmt-vet-tidy` (all module legs).
- [ ] `CHANGELOG.md`: rename `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD` and
      update the compare links at the bottom. It covers the core module;
      adapter changes go in the adapter release notes.
- [ ] A minor release must be additive. Compare the exported API with the
      previous tag:
      ```
      go install golang.org/x/exp/cmd/apidiff@latest
      git worktree add /tmp/aerr-prev vPREV
      (cd /tmp/aerr-prev && GOWORK=off apidiff -w /tmp/prev.apidiff .)
      GOWORK=off apidiff /tmp/prev.apidiff .
      ```

## Step 1 — tag the core

```
VERSION=vX.Y.Z
git checkout main && git pull --ff-only
git tag -a "$VERSION" -m "$VERSION" && git push origin "$VERSION"
```

## Step 2 — PR: point the adapters at it (only when they need it)

In `zerolog/` and `zap/`, run `go get github.com/tafaquh/aerr@$VERSION`
and `go mod tidy`. Then run `go mod tidy` in `benchmarks/` and
`examples/` as well: their `replace` directives make the new requirement
ripple into their go.mod files, and `fmt-vet-tidy` fails until it is
recorded. Merge once `consumer-smoke` is green: that is the proof that
`go get` resolves and compiles for consumers.

## Step 3 — tag the adapters

```
git checkout main && git pull --ff-only
git tag -a zerolog/vX.Y.Z -m zerolog/vX.Y.Z
git tag -a zap/vX.Y.Z -m zap/vX.Y.Z
git push origin zerolog/vX.Y.Z zap/vX.Y.Z
```

## Step 4 — GitHub releases

The core's notes are its `CHANGELOG.md` section:

```
NOTES="$(awk -v v="${VERSION#v}" 'index($0, "## [" v "]") == 1 {f=1; next} /^## \[/ || /^\[.*\]:/ {f=0} f' CHANGELOG.md)"
gh release create "$VERSION" --title "$VERSION" --notes "$NOTES"
```

Adapter releases (`gh release create zerolog/vX.Y.Z ...`) list the
adapter's own changes, since the core changelog does not cover them.

## Rollback

Never delete or move a pushed tag: the module proxy has cached it.
Publish a `retract` directive in the next release's go.mod instead (as
`zerolog/go.mod` does for `v1.0.0`) and cut a new version.
