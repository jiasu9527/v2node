# Pinned Xray dependency

`xray-core/` is a source snapshot of `github.com/jiasu9527/xray-core v1.260327.0-jiasu3`
(the module path remains `github.com/xtls/xray-core`). Root `go.mod` replaces Xray
with this directory, so normal `go test`, `go build`, Docker and release CI all
compile the same lifecycle fixes. Do not edit the global Go module cache.

The original MPL-2.0 license and upstream sources are retained. The original
module's per-file SHA-256 manifest is `xray-core.base-sha256.json` for auditing.
Local changes are limited to `proxy/anytls/{inbound,user,session,stream}.go` and
`proxy/anytls/lifecycle_test.go`. No unrelated Xray behavior is upgraded.

The copy is intentional: these fixes must be reproducible before a new upstream
fork version is published. When replacing the local snapshot with a released
module, verify all lifecycle tests below against that exact release first.

From the v2node repository root:

```sh
GOEXPERIMENT=jsonv2 go test -race github.com/xtls/xray-core/proxy/anytls -count=20
```

Root `go test ./...` does not discover a nested module's own tests; release CI
therefore runs this package explicitly using the root dependency graph.
