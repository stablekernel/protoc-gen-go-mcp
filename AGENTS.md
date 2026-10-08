# AGENTS.md

Guidance for coding agents (and humans) working in this repository. See
[`docs/GOAL.md`](docs/GOAL.md) for what we're building and what's out of scope,
and [`CONTRIBUTING.md`](CONTRIBUTING.md) for the human process.

## What this is

`protoc-gen-go-mcp` is a `protoc` plugin. For each gRPC service in a `.proto`
file it generates `<file>_mcp.pb.go`: an MCP (Model Context Protocol) server
that exposes each non-streaming RPC as an MCP tool and forwards tool calls to
a gRPC client.

- `cmd/protoc-gen-go-mcp/`: the plugin. `main.go` is the protogen entry point,
  and `mcp.go` emits the generated code (with `g.P` calls, not templates).
- `examples/protos/example.proto`: the example `VibeService`.
- `examples/gen/example/v1/`: the code generated from it, committed.
  `example_mcp.pb.go` is this plugin's output.
- `cmd/mcp-vibe/`: a demo MCP server over stdio, backed by an in-process gRPC
  server for the example service.

## Checks

Run all of these before opening a pull request, and paste the commands and
their results in the PR:

```sh
gofmt -l .                # must print nothing (until #86 lands, ignore test/snapshots/)
go vet ./...
go build ./...
go test ./...
```

If you changed the generator (`cmd/protoc-gen-go-mcp/**`) or the example
proto, regenerate the committed examples and commit the result:

```sh
make generate   # needs protoc 29.3, protoc-gen-go v1.36.6, protoc-gen-go-grpc v1.5.1 on PATH
git status      # the regenerated files belong in the same PR
```

Once the golden-file test exists, update it with
`go test ./cmd/protoc-gen-go-mcp -run Golden -update` and review the diff;
an unexpected change in generated code is a bug until explained.

## Conventions

- **Pull request titles must be Conventional Commits** (enforced by CI;
  release-please builds the changelog from them): `feat:`, `fix:`, `docs:`,
  `test:`, `ci:`, `refactor:`, `perf:`, `chore:`, `revert:`. Use `feat!:` or
  `fix!:` and a `BREAKING CHANGE:` paragraph in the body for breaking changes
  to the generated API.
- Reference the issue in the PR body (`Closes #N`).
- **Never edit generated files by hand** (`*.pb.go`); change the generator or
  the proto and regenerate.
- Don't touch `CHANGELOG.md`, `.release-please-manifest.json` or
  `release-please-config.json`, and don't bump versions: release-please owns
  them.
- Don't change `.github/workflows/**` unless the issue asks for it.
- Keep dependencies minimal; prefer the standard library,
  `google.golang.org/protobuf` and the MCP SDK. Pin versions in `go.mod` and
  commit `go.sum`.
- Generated code must compile without warnings from `go vet`, and its
  exported API is public API: document any change to it in the PR.
