# Go Zero Hexagon PR demo: reuse one image and Go caches

## Intent and boundary

For the `aayushhzz/go-zero` fork's PR demo, keep Hexagon's strict candidate subset and reduce repeated CodeBuild setup/compilation time. Do not rebuild a container image in PR builds or execute the full test suite. This is a diagnostic subset demo, not a claim that unselected tests are safe to omit.

The current selected package, `core/stores/monc`, contains only `cachedmodel.go` and `cachedmodel_test.go`. `go test -run` already compiles just that one test file in the package, while compiling its production code and imported dependencies. A literal test-file-only build is not valid Go testing; replacing package-mode `go test` with an explicit file list would not remove any other `monc` test files and can alter package semantics. Preserve the existing selected-root command.

## Design

Keep the existing AWS-managed `aws/codebuild/standard:7.0` image and Go 1.24 runtime, with the prebuilt Hexagon selector fetched from S3. Pin the CodeBuild project to that managed image for the demo. The PR buildspec must not contain `docker build`, `go build ./...`, or `go test ./...`; it computes the PR diff and invokes `ci-impact --require-strict-subset`, which runs selected roots only.

Configure CodeBuild's managed **S3 cache** for the Go module download directory and Go compiler cache. Give the project a dedicated cache prefix and narrowly scoped read/write permissions on that prefix, distinct from the read-only selector-binary prefix. In the buildspec, set stable absolute `GOMODCACHE` and `GOCACHE` paths and enumerate both as cache paths. Key by a Go 1.24 Linux/amd64 prefix plus hashes of `go.mod` and `go.sum`; Go's own build cache invalidates changed source and toolchain inputs. Do not key the cache by each PR SHA, which would eliminate cross-PR reuse. S3 persists across build hosts, unlike best-effort local custom caching; Docker-layer cache is unnecessary because no image is built. Large S3 cache transfer can cost more time than it saves, so measure before claiming a speedup.

Only the dedicated Go Zero CodeBuild project and fork buildspec change. Do not change Prometheus, Kubernetes, the shared Hexagon runner, or the PR's example production-code change. New branches will carry the buildspec change; adjust the live project's image/cache settings only after reading back its current configuration and IAM scope. Preserve its PR webhook.

## Behavior and validation

On a cache hit, CodeBuild restores dependencies/build artifacts, the selector computes the diff and candidate roots, and Go rebuilds only invalidated package artifacts before running the selected tests. Hexagon may still scan repository sources to plan the candidates; caching the Go compiler does not remove that planning work. On a cache miss or changed dependencies, Go still compiles/downloads the required dependencies; the job never silently claims file-only compilation. If strict selection fails, the demo build fails rather than expanding to an unreported full suite.

Validate buildspec syntax and current project settings; verify the cache prefix IAM policy, the unchanged PR webhook, and the effective image/cache settings by readback. Run two equivalent PR builds after cache warm-up, checking cache hit/miss logs, selected roots, passed tests, and separate CodeBuild phases versus selector/test command durations. Compare against the prior no-cache run but report cold/warm and cache transfer overhead separately. Do not claim a percentage improvement until timings are observed. CodeBuild's phase named `build` remains present even though it does not build an image.

AWS reference: https://docs.aws.amazon.com/codebuild/latest/userguide/build-caching.html (S3 cache shared across hosts; local cache host-local).
