# Releasing

This repo publishes three independently-versioned Go modules from one
`go.work` workspace: the root `github.com/tafaquh/aerr`, and the two
adapters, `github.com/tafaquh/aerr/zerolog` and `github.com/tafaquh/aerr/zap`.
The adapters `require` a specific **published** root version — not the
workspace-local checkout — so the release order is not arbitrary. Follow
this sequence exactly.

## Why the order matters (the chicken-and-egg constraint)

- `zerolog/go.mod` and `zap/go.mod` currently `require
  github.com/tafaquh/aerr v1.0.0`. The published `v1.0.0` tag declares
  `go 1.24.7` in its own go.mod (see `git show v1.0.0:go.mod`), even
  though the root module's go.mod on `main` has since been lowered to
  `go 1.21`. Go requires a module's `go` directive to be at least as high
  as every module it requires (as resolved by MVS), so both adapters —
  and `go.work` itself — are pinned at `go 1.24.7` today purely because
  of that stale published dependency, not because they need anything
  from Go 1.24.
- The adapters cannot lower their own `go` directive to 1.21 until they
  depend on a **published** aerr release that itself declares `go 1.21`.
  `main`'s go.mod already says 1.21, but that's only real for consumers
  once it's tagged and pushed — `go get`/`go mod tidy` resolve against
  the module proxy, not this working tree.
- Consequently: the root module must be tagged and pushed *first*. Only
  then can the adapters bump their `require` line to the new version and
  drop their `go` directive in the same PR.

## Preflight checklist

Run this before tagging anything:

- [ ] `main` is green on every CI job: the `test` matrix (both Go
      versions), `consumer-smoke`, `lint`, and `fmt-vet-tidy` (all five
      module legs).
- [ ] Consumer-view build, no workspace, no `replace` directives:
      ```
      GOWORK=off go build ./...
      GOWORK=off go test ./...
      ```
      run from the repo root. This is what `go get github.com/tafaquh/aerr`
      actually sees.
- [ ] `go vet ./...` is clean in every module: `.`, `zerolog`, `zap`,
      `benchmarks`, `examples`.
- [ ] Benchmarks compile and smoke-run (not a performance measurement,
      just proves they build against the release candidate):
      ```
      cd benchmarks && go test -run=NONE -bench=. -benchtime=1x ./...
      ```
- [ ] Examples build: `cd examples && go build ./...`.
- [ ] `CHANGELOG.md` has a finalized `[1.1.0] - YYYY-MM-DD` section (not
      `[Unreleased]`) and the compare links at the bottom of the file are
      correct for this release.
- [ ] API diff against the previous tag, if `apidiff` is available,
      to confirm nothing incompatible slipped into a minor release:
      ```
      go install golang.org/x/exp/cmd/apidiff@latest
      apidiff -w /tmp/aerr-v1.0.0.apidiff github.com/tafaquh/aerr@v1.0.0
      apidiff /tmp/aerr-v1.0.0.apidiff github.com/tafaquh/aerr@.
      ```
      Investigate (and, if warranted, retarget the version bump) if this
      reports incompatible changes — a minor release must be additive.

## Step 1 — Tag & push `v1.1.0` (root module)

```
git checkout main && git pull --ff-only
git tag -a v1.1.0 -m "v1.1.0"
git push origin v1.1.0
```

This publishes the `go 1.21` directive to the module proxy. Nothing in
step 2 is possible before this tag exists upstream.

## Step 2 — PR: point the adapters at the new release

On a branch off `main`:

1. In `zerolog/`: `go get github.com/tafaquh/aerr@v1.1.0`, then lower
   `go 1.24.7` to `go 1.21` in `zerolog/go.mod` (now legal, since
   `v1.1.0`'s own go.mod declares 1.21). Run `go mod tidy`.
2. In `zap/`: same two edits (`go get` the new require, lower the `go`
   directive), then `go mod tidy`.
3. In `go.work`: lower `go 1.24.7` to `go 1.21` — it no longer needs to
   accommodate an adapter pinned above the root's floor — and update its
   explanatory comment.
4. Run `go mod tidy` in root as well for good measure, and re-run the
   full preflight checklist above against this branch.
5. Open the PR and confirm CI is green, in particular the
   `consumer-smoke` job: it builds and tests each adapter with
   `GOWORK=off` against the just-published `v1.1.0` pulled from the
   module proxy (no `replace` directive), which is exactly the check
   that would have caught the broken `zerolog v1.0.0` release. A green
   `consumer-smoke` run here is the proof that `go get
   github.com/tafaquh/aerr/zerolog@v1.1.0` (once tagged in step 3) will
   actually resolve and compile standalone.
6. Merge to `main`.

## Step 3 — Tag & push the adapter modules

```
git checkout main && git pull --ff-only
git tag -a zerolog/v1.1.0 -m "zerolog/v1.1.0"
git tag -a zap/v1.0.0 -m "zap/v1.0.0"
git push origin zerolog/v1.1.0 zap/v1.0.0
```

- `zerolog` jumps from `v1.0.0` (retracted) straight to `v1.1.0`, in
  step with the aerr release it now requires. Pushing this tag is also
  what makes the `retract v1.0.0` directive already committed in
  `zerolog/go.mod` take effect for consumers: `go get
  github.com/tafaquh/aerr/zerolog` will skip the retracted `v1.0.0` and
  resolve straight to `v1.1.0`.
- `zap` has never been tagged before; `v1.0.0` is its first published
  release.

## Step 4 — Create GitHub releases

Create a release for each of the three tags and paste in the relevant
`CHANGELOG.md` section as the release notes body:

```
NOTES="$(awk '/^## \[1\.1\.0\]/{flag=1} /^## \[/ && !/^## \[1\.1\.0\]/{flag=0} /^\[.*\]:/{flag=0} flag' CHANGELOG.md)"
gh release create v1.1.0 --title v1.1.0 --notes "$NOTES"
gh release create zerolog/v1.1.0 --title zerolog/v1.1.0 \
  --notes "See github.com/tafaquh/aerr v1.1.0 changelog (this repo's changelog is not split per module)."
gh release create zap/v1.0.0 --title zap/v1.0.0 \
  --notes "First release of the zap adapter. See github.com/tafaquh/aerr v1.1.0 changelog."
```

The `awk` extracts everything from the `## [1.1.0]` heading up to (but
not including) either the next `## [`-heading or the reference-style
compare links at the bottom of the file — it does not depend on a
`## [1.0.0]` heading existing, since today's changelog has none (the
project's changelog starts at `[Unreleased]`/`[1.1.0]`).

## Rollback

Pushed tags are effectively permanent once the module proxy has cached
them — do not delete or move a tag that a consumer may already depend
on. If a release turns out broken, publish a `retract` directive for it
in the next release's go.mod (as already done for `zerolog v1.0.0`) and
cut a new patch/minor version, rather than rewriting history.
