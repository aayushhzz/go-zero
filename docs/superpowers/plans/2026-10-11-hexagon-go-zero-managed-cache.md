# Go Zero Managed Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rerun the Go Zero Hexagon PR demo with selected tests and reusable CodeBuild-managed Go caches, without rebuilding an image.

**Architecture:** Keep `aws/codebuild/standard:7.0` and the prebuilt Hexagon binary. Add S3-backed module/compiler cache paths to the fork's PR buildspec, grant cache-prefix S3 access to only the Go Zero CodeBuild role, and set the existing project's cache type to S3. Use a PR with the same `DelCache` demonstration change to exercise the PR webhook and report cold/warm timings.

**Tech Stack:** Go 1.25, Hexagon `ci-impact`, AWS CodeBuild, S3 cache, IAM, GitHub PR.

**Spec:** `docs/superpowers/specs/2026-10-11-hexagon-go-zero-managed-cache-design.md`

## Global Constraints

- Scope only `aayushhzz/go-zero` and the `hexagon-go-zero-pr` CodeBuild project in `us-west-2`.
- Preserve the `--require-strict-subset` candidate-only behavior and PR webhook; no full-suite test or Docker image build in the buildspec.
- Reuse the existing AWS-managed `aws/codebuild/standard:7.0` image and prebuilt Hexagon selector.
- Use a managed S3 cache rather than host-local best-effort cache; measure restore/save overhead before claiming improvement.
- `core/stores/monc` has one `_test.go` file. Its production code and imports still need compilation.

## Review Focus

- Different `go.sum` or `go.mod`: cache key changes, test still runs and cache populates.
- Missing/denied S3 cache: build fails visibly or logs a cache miss; do not report a warm hit.
- Missing candidate roots: strict subset fails rather than silently running a full suite.
- Non-PR/manual trigger: existing webhook guard rejects it unless an explicit diagnostic override is used and labeled.
- Cache transfer slower than compilation: report both durations and do not claim speedup.

---

### Task 1: Buildspec cache wiring and demo source

**Files:** Modify `buildspec.hexagon-pr.yml`; modify `core/stores/monc/cachedmodel.go` with the same behavior-preserving `DelCache` demonstration change as PR #1; test the buildspec with a YAML parser and focused Go test.

**Interfaces:** Consumes the CodeBuild S3 cache location `hexagon-ci-demo-029136913161-usw2/cache/go-zero-pr`; produces stable absolute Go cache paths and a cache key reused by PRs with matching `go.mod`/`go.sum`.

- [ ] **Step 1: Confirm the old buildspec has no cache block and the demonstration function returns `mm.cache.DelCtx` directly.** Run `rg -n 'cache:|GOCACHE|GOMODCACHE|func \(mm \*Model\) DelCache' buildspec.hexagon-pr.yml core/stores/monc/cachedmodel.go` and inspect the function body.
- [ ] **Step 2: Add the minimal buildspec configuration.** Select `golang: 1.25` in the existing managed image; add `GOMODCACHE: /root/.cache/hexagon-go-zero/mod` and `GOCACHE: /root/.cache/hexagon-go-zero/build` under `env.variables`, then append:

  ```yaml
  cache:
    key: go125-linux-amd64-$(codebuild-hash-files go.mod go.sum)
    paths:
      - '/root/.cache/hexagon-go-zero/mod/**/*'
      - '/root/.cache/hexagon-go-zero/build/**/*'
  ```

  Change only `DelCache` to `err := mm.cache.DelCtx(ctx, keys...); return err` so the PR diff has the same selected roots. Do not change the `ci-impact --require-strict-subset` invocation.
- [ ] **Step 3: Validate.** Run `/usr/bin/ruby -e 'require "yaml"; YAML.load_file("buildspec.hexagon-pr.yml")'`, `git diff --check`, and `go test -count=1 -run '^TestModel_DelCache$' ./core/stores/monc` with Go 1.25; record outcome and duration separately from downloads.
- [ ] **Step 4: Commit with sign-off.** Stage only the buildspec and demonstration source change; commit `ci: cache Go artifacts for Hexagon PR demo`.

### Task 2: Restrict IAM and enable the managed project cache

**Files:** No repository files; modify only the existing inline `HexagonGoZeroCI` policy on `hexagon-go-zero-codebuild` and `hexagon-go-zero-pr` cache configuration.

**Interfaces:** Consumes Task 1 cache paths/key; produces a cache in the existing bucket under `cache/go-zero-pr`, while preserving the current image, source, buildspec, webhook, and log/binary permissions.

- [ ] **Step 1: Read back current state.** Verify AWS STS account `029136913161`, `batch-get-projects --names hexagon-go-zero-pr`, `iam get-role-policy`, and `s3api get-bucket-location`; stop before mutation if any identity, image, region, or role differs.
- [ ] **Step 2: Add narrowly scoped permissions.** Preserve the existing policy statements and add `s3:GetObject`/`s3:PutObject` for `arn:aws:s3:::hexagon-ci-demo-029136913161-usw2/cache/go-zero-pr/*`; add bucket-level read/list only if AWS's cache API demonstrably requires it, conditioned on that prefix. Submit the complete policy document using `aws iam put-role-policy` and read it back.
- [ ] **Step 3: Configure CodeBuild.** Run `aws codebuild update-project --name hexagon-go-zero-pr --cache type=S3,location=hexagon-ci-demo-029136913161-usw2/cache/go-zero-pr` in the verified burner session. Read back `environment.image`, `cache`, `source`, and webhook settings; verify no unrelated project field changed.
- [ ] **Step 4: Validate settings before a build.** Read the updated inline policy and project cache with AWS CLI; if fields differ from the intended cache prefix or the prior image/source, correct only those fields before triggering Task 3.

### Task 3: PR, repeated builds, and report

**Files:** No additional repository files; GitHub PR from `aayush/go-zero-file-cache-demo` into `aayush/hexagon-ci-pr-base`.

**Interfaces:** Consumes Tasks 1–2; produces build IDs, exact selected-root count, test status, cache logs, build-phase durations, and a comparison with the earlier no-cache run.

- [ ] **Step 1: Push the demo branch with `git push -u origin aayush/go-zero-file-cache-demo` and open the PR with `gh pr create --base aayush/hexagon-ci-pr-base --head aayush/go-zero-file-cache-demo`.** Body explicitly says candidate-only and no full-suite proof; link the design spec and include the fork's normal release-note format if required.
- [ ] **Step 2: Let the PR webhook trigger CodeBuild; check its source SHA and cache-miss/warm state.** Use `aws codebuild batch-get-builds` and CloudWatch log events. If access is denied, adjust only the precise missing IAM permission; do not grant bucket-wide write or treat a red build as a timing success. Run a second equivalent build on the same PR revision through the existing webhook/retrigger mechanism, labeling any manual override honestly.
- [ ] **Step 3: Confirm all candidate roots pass and record command durations.** Report cache restored/saved bytes or log evidence, phases (download/install/build/finalizing), selected root count, and separate selector/test elapsed time. A cache-hit result is not established by the presence of a cache key alone.
- [ ] **Step 4: Compute a like-for-like time comparison.** Compare cold versus warm and earlier no-cache runs using the same revision/compute type where possible; compute `% reduction = (baseline - cached) / baseline * 100`, isolating setup/build from test execution and labeling any incomparable runs.
- [ ] **Step 5: Final review and report.** Inspect the diff for secrets/unrelated changes, report any failed test or cache miss honestly, and link the PR and CodeBuild run(s).
