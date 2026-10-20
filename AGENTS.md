# AGENTS.md

## Project
- Go backend service
- Prefer standard library first. Only add third-party deps with explicit approval.
- Approved deps (2026-10-20): `github.com/decred/dcrd/dcrec/secp256k1/v4` (secp256k1 signing),
  `golang.org/x/crypto` (legacy Keccak-256), `github.com/coder/websocket` (WebSocket client for
  Alchemy subscriptions and the sequencer feed).

## Commands
- Format: `gofmt -w .` or `go fmt ./...`
- Vet: `go vet ./...`
- Test: `go test -race ./...`
- Lint: `golangci-lint run` (if configured)

## Go Style (must follow)
- Use `gofmt` / `goimports`. No style debates.
- Package names: short, lowercase, single word (no underscores).
- Exported names: MixedCaps. Acronyms: `userID`, `httpClient` (not `userId`).
- Receivers: 1–2 letters (`s`, `u`). Never `self` or `this`.
- Errors: always handle. Never `_ = fn()`. Wrap with `fmt.Errorf("context: %w", err)`. Use `errors.Is` / `errors.As`.
- Context: first parameter for any function that can block or is long-running.
- Interfaces: small (1–3 methods), defined at the call site. Accept interfaces, return concrete types.
- Concurrency: every goroutine must have a clear exit path (context cancellation). Prefer channels for communication, mutex for shared state.
- Testing: table-driven tests with `t.Run`. Prefer real interfaces over heavy mocks.
- No global mutable state. No `init()` for business logic.
- Prefer modern idioms available in the project's Go version (from go.mod).

## Architecture
- Source of truth: `docs/ARCHITECTURE.md` (hexagonal / ports & adapters).
- Dependencies point inward: `internal/adapter/*` imports `internal/domain/port`;
  `internal/domain` never imports adapters or protocol libraries.
- `cmd/*` is the only composition root that knows concrete implementations.
- Adapters never import each other; shared EVM primitives live in `internal/adapter/out/chain/evm`.
- Domain services reach storage only through outbound ports (`PositionStore`, `DeployerStore`, ...).
- Hot path (launch → decide → sign → submit) does no disk I/O; journaling is non-blocking.
- Keep changes minimal and focused.

## Must Never
- Ignore errors with `_`
- Use `interface{}` when generics or concrete types work
- Start goroutines without termination condition
- Add dependencies without stating why

## Module Design (must follow)

- 简洁性 (simplicity): one package = one responsibility; no dead or speculative
  code. Prefer the simplest data flow (a function call when a single goroutine
  owns the state, a channel only for real cross-goroutine signalling).
- 解耦性 (decoupling): modules talk through small interfaces defined at the
  call site, not through concrete types. Business strategies implement
  `strategy.Strategy` and register by name (v1: `ponsv2` only); shared
  services (`risk`, `timing`, `positions`, `monitor`) expose ports, not internals.
- 可拓展性 (extensibility): a new capability is a new registered implementation
  (chain by chainID, venue by `chainID:venueName`, `strategy.Strategy`,
  `protectors.Protector`), never an edit to the core engine. Keep `engine` thin:
  it wires, it does not implement business logic.
- 范围 (scope): v1 targets Robinhood Chain + Pons V2 only. Do not add code for
  other chains, launchpads, or Solana until `docs/ARCHITECTURE.md` schedules it.
- 基础设施 (infrastructure): no self-hosted chain node. Use official endpoints
  (sequencer, feed, public RPC) and Alchemy only.
- Monitor is read-only: it consumes events and never sends trading commands.
  Business modules are independent in code, but share account-level risk; that
  is why risk is a cross-cutting aggregator, not a per-module feature.
