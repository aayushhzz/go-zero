# Flow-inspired isolated E2E fixture

Flow's workflows E2E setup starts MongoDB as a replica set, Redis, Kafka and many private DevRev services. This public fixture keeps the dependency startup and health-gate pattern, with a small API and worker using live MongoDB and Redis. Kafka starts and reaches healthy state, but the current request path does not publish to it. It does not claim equivalence with Flow's private-service E2E suite.

The API inserts a job in MongoDB and queues its ID in Redis. The worker consumes the queue, updates MongoDB and writes a Redis completion marker. `TestFlowlikeE2E` checks the request-to-worker path and the completion marker. The test skips outside the isolated stack; the E2E buildspec sets `FLOWLIKE_E2E=1` explicitly.

For a manual AWS CodeBuild run, push this branch to the fork and start `hexagon-go-zero-pr` at its exact commit SHA with `buildspec.flowlike-e2e.yml` as `buildspecOverride` and `privilegedModeOverride=true`. The existing PR project settings stay unchanged. The Compose file uses only prebuilt dependency images; CodeBuild builds a single Go binary for both application roles and never invokes `docker build`. It still compiles the changed Go package and dependencies and pulls service images on a fresh worker. A separate Hexagon graph plan and a safety gate are needed before running a smaller set of E2E test roots on a future change.
