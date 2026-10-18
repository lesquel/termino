# Contributing to termino

Thanks for helping improve termino. This guide covers how to build it, run the
same checks as CI, and send a change that is easy to review.

## Quick path

1. Fork [lesquel/termino](https://github.com/lesquel/termino) (or create a
   branch if you have write access) and clone it.
2. Make a small, focused change with tests next to the code.
3. Run `make lint vet race build` and make sure it passes.
4. Commit with a [Conventional Commit](#commit-messages) message and open a
   pull request against `main`.

By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
Security issues go through [SECURITY.md](SECURITY.md), never a public issue.

## Prerequisites

| Tool | Version | Needed for |
|---|---|---|
| Go | 1.25.5 or newer (see `go.mod`) | Building and testing |
| git | any recent version | Cloning, board sharing tests |
| C toolchain (gcc or clang) | any | Only `make race` (the race detector needs cgo) |
| [GoReleaser](https://goreleaser.com) v2 | optional | Only `make release-snapshot` |

termino itself needs no cgo: `modernc.org/sqlite` is pure Go.

## Build and run locally

```sh
git clone https://github.com/<you>/termino.git
cd termino
make build            # writes ./bin/termino with the version injected
./bin/termino version
```

The code is organised by feature using Clean Architecture. See
[Architecture](README.md#architecture) in the README for the layout.

### Use a sandbox, not your real data

termino stores its database, config and logs under the XDG directories, and
`connect`/`setup` write agent config under `$HOME`. When you try a local build,
point all of them at a temporary directory so your real board stays untouched:

```sh
sandbox="$(mktemp -d)"
mkdir -p "$sandbox/home"
tt() {
  env HOME="$sandbox/home" \
    XDG_CONFIG_HOME="$sandbox/config" \
    XDG_DATA_HOME="$sandbox/data" \
    XDG_STATE_HOME="$sandbox/state" \
    "$PWD/bin/termino" "$@"
}
```

Run `rm -rf "$sandbox"` when you are done.

### Try the TUI

```sh
tt setup    # setup wizard
tt          # open the board
tt doctor   # check the install, database and integrations
```

Logs go to `$sandbox/state/termino/termino.log`, never to stdout.

### Try the MCP server

`termino mcp` speaks JSON-RPC over stdio. Pipe a few requests in to check that
it starts and lists its tools:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"manual","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | tt mcp
```

Add `--read-only` to expose only the read tools. The server must never print
anything else to stdout: that would break the protocol.

## Run the checks

CI (`.github/workflows/ci.yml`) runs these steps on every push to `main` and
every pull request. Run them before you push:

| CI step | Command | Make target |
|---|---|---|
| gofmt | `gofmt -l .` must print nothing | `make lint` |
| vet | `go vet ./...` | `make vet` |
| test (race) | `go test -race ./...` | `make race` |
| build (no cgo) | `CGO_ENABLED=0 go build ./cmd/termino` | `make build` |

`make test` runs `go test ./...` without the race detector, which is faster
while you iterate. `make bench` runs the latency benchmarks in
`internal/bench`. `make all` runs lint, vet, test and build.

To fix formatting, run `gofmt -w .` (or `gofmt -w <file>`).

## Testing rules

- Put tests next to the code they cover (`foo.go` and `foo_test.go`).
- Tests must never read or write your real config, data or home directory.
  Use `t.TempDir()` and set `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME` and
  `XDG_STATE_HOME` to paths inside it. `isolatedEnv` in
  `internal/cli/runtime_integration_test.go` shows the pattern for tests that
  run the binary.
- For a bug fix, write the failing test first, watch it fail, then fix the
  code and watch it pass.
- New behavior comes with tests in the same pull request.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org):
`<type>(<optional scope>): <summary>`, imperative mood, lowercase summary.

| Type | Use for |
|---|---|
| `feat` | A new user-facing capability |
| `fix` | A bug fix |
| `docs` | Documentation only |
| `test` | Tests only |
| `refactor` | Code change with no behavior change |
| `ci` | Workflows and release config |
| `chore` | Maintenance that fits none of the above |

Examples from the history:

```text
feat(share): import .termino changes automatically
fix(tui): keep board footer after closing a dialog
test(cli): run integration commands outside the repository
ci: publish releases without draft so install.sh finds latest
```

Do not add `Co-Authored-By` trailers or AI attribution lines. Keep tests and
docs in the same commit as the behavior they cover.

## Branches and pull requests

1. Branch from an up-to-date `main` (for example `fix/widget-zsh-prompt`).
2. Keep each pull request small and focused on one change. Split large work
   into a series of pull requests.
3. Rebase on `main` instead of merging `main` into your branch.
4. Fill in the pull request template, including the linked issue.
5. CI must pass before review.
6. Pull requests are merged with rebase, so `main` keeps a linear history and
   every commit on your branch should stand on its own.

## Report a bug or propose a feature

Open an issue with one of the
[issue forms](https://github.com/lesquel/termino/issues/new/choose):

- **Bug report**: include the output of `termino version` and
  `termino doctor`, your OS and architecture, terminal, install method, and
  the steps to reproduce.
- **Feature request**: describe the problem first, then the change you have
  in mind.

For larger features, open an issue to agree on the design before you write the
code.

## Add an agent connector

A connector installs and removes the termino integration for one AI agent.

1. Implement the `Connector` interface from
   `internal/features/connector/domain/connector.go` (`Agent`, `Detect`,
   `Connect`, `Disconnect`, `Owns`; `Describer` is optional).
2. Put the implementation in
   `internal/features/connector/infrastructure/<agent>/`. The Kiro connector
   in `internal/features/connector/infrastructure/kiro/connector.go` is the
   reference, with tests in the same directory.
3. Register it in every place that lists connectors:

| File | What to change |
|---|---|
| `internal/cli/connect.go` | Add the agent to `supportedAgents` and a case to `resolveConnector` (this also covers `termino uninstall`) |
| `internal/cli/setup.go` | Add it to the `connectors` list passed to the setup wizard |
| `internal/cli/doctor.go` | Add it to the `connectors` list that `termino doctor` checks |

4. Test `Connect` and `Disconnect` against a temporary `HOME`, including that
   disconnecting leaves other entries in shared config files untouched.
5. Document the new `termino connect <agent>` in the README.

## Release process (maintainers)

Pushing a `v*` tag runs `.github/workflows/release.yml`. GoReleaser
(`.goreleaser.yaml`) runs the tests, builds the archives and `checksums.txt`,
and publishes the GitHub release directly (no draft). Tags with a suffix such
as `v1.2.0-rc1` become pre-releases, which `install.sh` skips.

1. Make sure `main` is green in CI. Optionally run `make release-snapshot` to
   build into `./dist` without publishing.
2. Tag and push:

   ```sh
   git checkout main && git pull
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

3. Wait for the release workflow to finish and check the release page.
4. Verify the published build through the installer:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/lesquel/termino/main/install.sh | TERMINO_VERSION=vX.Y.Z sh
   termino version
   ```

   `termino version` must print the new version.
