# Retirement dependency verification

At base `bdaea6a`, the infrastructure module already selects the fixed
OpenTelemetry releases for the three reported Go vulnerabilities:

- `GO-2026-6615`: `go.opentelemetry.io/otel/sdk/log` `v0.21.0`.
- `GO-2026-6508`: `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`
  `v0.21.0`.
- `GO-2026-6505`: `go.opentelemetry.io/otel/exporters/otlp/otlptrace`,
  `otlptracegrpc`, and `go.opentelemetry.io/otel/sdk` `v1.45.0`.

The Go vulnerability database lists those as the fixed releases. The module
graph resolves them at those versions. The API, metric, and trace modules
resolve at `v1.47.0`; OpenTelemetry modules version independently, so forcing
those modules down to `v1.45.0` is unnecessary for the reported fixes. No
`go.mod` or `go.sum` edit is warranted.

Verification run from `infra/` on `bdaea6a`:

```sh
GOMODCACHE=/Volumes/BuildOffload/go/pkg/mod GOPATH=/Volumes/BuildOffload/go go list -m all
```

The command resolved the module graph using the external volume's module cache.
No build or tests were run; the shared host's one-minute load was above the
repository's limit of 10.
