# Tor/I2P Migration Plan (AI.md PART 31 rewrite + PART 12 re-verify)

Scope: AI.md PART 31 was restructured into **31.1 Tor** (ADD_ONION → torrc
`HiddenServiceDir`/`HiddenServicePort` + PROXY-protocol backend) and a brand
new **31.2 I2P** (optional eepsite, i2pd or SAM). PART 12 (Tor HTTP semantics)
is unchanged in substance and already implemented correctly — re-verify only,
no rewrite. Rest of the ~1189-line AI.md diff is out of scope for this pass;
anything else found will be logged to TODO.AI.md, not fixed here.

**Hard constraint: NO REGRESSIONS.** The biggest risk is silently changing
the `.onion` address of every existing deployment during the key-format
migration. That gets an explicit conversion step and a real-Tor-binary
verification test before it's trusted.

## 1. `src/tor/tor.go` rewrite

Preserve the public `Manager` API unchanged (`Start, Close, Running,
OnionAddress, GetHTTPClient, Restart, UpdateConfig, RegenerateAddress,
ApplyKeys, Monitor`) so `server.go` needs almost no changes. Preserve
`FindBinary`/`findInPath`/`ensureTorDirs` as-is (already spec-compliant).

- `NewManager(ctx, serverPort, cfg, handler http.Handler)` — add `handler`
  param (the app's `s.router`), stored for the backend HTTP server.
- `getTorConfig(cfg *Config, backendPort int) string` — rewrite to emit:
  - `HiddenServiceDir {data_dir}/tor/site`
  - `HiddenServiceVersion 3`
  - `HiddenServicePort {virtual_port} 127.0.0.1:{backendPort}`
  - `HiddenServiceExportCircuitID haproxy`
  - `VanguardsLiteEnabled 1`
  - `HiddenServiceSingleHopMode 0`
  - Keep existing SOCKS/ControlPort/SafeLogging/bandwidth/accounting/
    no-relay/startup lines unchanged (already verified spec-correct).
  - Remove the stale "created via ADD_ONION" doc comment.
- Torrc is regenerated unconditionally on **every** startup (drop the
  `os.IsNotExist`/write-if-changed gating in `updateTorrc`/`writeIfChanged`).
- `startLocked()` rewrite:
  1. `ensureTorDirs` (unchanged).
  2. **Key migration** (see §2) — run before Tor starts.
  3. Allocate backend listener: `net.Listen("tcp", "127.0.0.1:0")` → get
     port → wrap with `proxyproto.Listener{Listener: ln}`.
  4. Start `http.Server{Handler: handler}` on that listener in a goroutine
     (own lifecycle, independent of the main server's TLS-aware
     `http.Server` objects in `server.go`).
  5. Write torrc with the allocated backend port.
  6. `binettor.Start(...)` (unchanged: ExePath, TorrcFile, DataDir,
     NoAutoSocksPort: true), `EnableNetwork` with bootstrap timeout +
     30s "connecting..." log (unchanged).
  7. **No `AddOnion` call.** Read the onion address from
     `{data_dir}/tor/site/hostname` (Tor writes this itself once the HS
     is published) — poll with short backoff up to bootstrap timeout.
  8. Optional outbound dialer if `UseNetwork` (unchanged).
- `OnionAddress()` — return the value read from `hostname` in step 7
  (cached in the `service` struct), not `serviceID + ".onion"`.
- `Close()` — also shut down the backend `http.Server` (graceful,
  short timeout) and close the backend listener, in addition to existing
  `t.Close()` behavior.
- `RegenerateAddress()` — now must delete the whole
  `{data_dir}/tor/site/` key material (`hs_ed25519_secret_key`,
  `hs_ed25519_public_key`, `hostname`), not just one file, then restart so
  Tor generates a fresh native-format identity.
- `ApplyKeys(keyData []byte)` — **contract change** (confirmed safe: zero
  existing callers via `grep -rn "ApplyKeys\|RegenerateAddress\|import-keys\|ImportKeys\|vanity" src/ --include=*.go | grep -v _test.go`,
  only the method definitions themselves and one unrelated paste-URL
  comment matched). New contract: `keyData` is Tor's **native on-disk
  secret-key file format** (the same bytes a vanity tool like `mkp224o`
  produces) — write directly to `hs_ed25519_secret_key` (0600), delete any
  stale `hostname`/public-key files so Tor regenerates them, restart.
- `saveKey()` (old blob-format writer) — remove; no longer applicable once
  `AddOnion` is gone.
- `Monitor()` — unchanged (only depends on the control connection).
- Drop `NumIntroPoints`-as-dead-field: either wire it into torrc
  (`HiddenServiceNumIntroductionPoints`, if that's a real Tor torrc
  directive) or confirm intentionally unused per spec and leave a note —
  verify against the reference torrc in AI.md PART 31.1 before deciding.

## 2. Key-format migration (critical regression-safety step)

Old format: `{data_dir}/tor/site/hs_ed25519_secret_key` was written by
`saveKey()` as raw base64-decoded bytes of the bine `ED25519-V3:<base64>`
blob (64-byte expanded ed25519 private key, **no header**).

Native Tor format: 96-byte file = 32-byte header
`"== ed25519v1-secret: type0 ==\0\0"` + the same 64-byte expanded key.

Migration step (runs once, at `startLocked()`, before Tor is spawned):
1. If `hs_ed25519_secret_key` exists and is exactly 64 bytes (old format,
   no header) → prepend the 32-byte native header, rewrite the file
   (0600), leave a `.migrated` marker or log line so it's not reprocessed.
2. If the file is already 96 bytes starting with the native header →
   leave untouched (already native, e.g. fresh install or already
   migrated).
3. If the file is some other size → do not touch; log a WARN (unexpected
   format, avoid corrupting an unknown state) and let Tor fail loudly
   rather than silently generating a new identity.

**Verification (mandatory, in Docker/Incus — never host):** write a test
that (a) starts a real `tor` binary against a manually-crafted old-format
key fixture, (b) captures the resulting `.onion` hostname, (c) independently
computes the expected onion address from the same key material via the
v3 onion address algorithm (or cross-checks against a known fixture pair
recorded ahead of time), (d) asserts they match. This is the one test that
actually proves "NO REGRESSIONS" for existing deployments — a code-review
read-through is not sufficient.

## 3. `server.go` changes (minimal, by design)

- Single call-site edit (~line 514): `tor.NewManager(ctx, serverPort,
  torCfg, s.router)`.
- No other call site changes — `TorRunning()`, `TorRestart()`,
  `TorOnionAddress()`, health/status endpoints, template data
  (`TorEnabled`/`TorRunning`/`TorAddress`), `persistOnionAddress()`,
  startup/shutdown sequence all keep working against the same `Manager`
  API surface.
- New: analogous I2P wiring (see §5) — new `i2pManager *i2p.Manager` field,
  construction, startup/shutdown, health/status exposure, template data
  (`I2PEnabled`/`I2PRunning`/`I2PAddress`).

## 4. New dependency

- Add `github.com/pires/go-proxyproto` (BSD-3-Clause) to `go.mod`/`go.sum`.
- Add a `LICENSE.md` third-party attribution entry: BSD-3-Clause requires
  the brief non-endorsement clause text per `project-rules.md`'s
  attribution table (not full license reproduction — the existing
  compact-table format for 10+ deps).

## 5. New I2P package `src/i2p/i2p.go`

Mirrors `tor.go`'s `Manager` structure. Opt-in: `server.i2p.enabled`
(default false) / `I2P_ENABLED` env / `--i2p` flag — no port, no config
file, no session unless enabled.

- `Config` struct: `Enabled, Binary, SAMAddress (default
  "127.0.0.1:7656"), VirtualPort (default 80), InboundLength,
  OutboundLength, InboundQuantity, OutboundQuantity (all default 3/3/5/5),
  SignatureType (default 7), BootstrapTimeout (default 5m), ConfigDir,
  DataDir`.
- Provider resolution order at `Start()`:
  1. Not enabled → no-op, return cleanly.
  2. Find `i2pd` binary (`cfg.Binary` → common locations → `$PATH`,
     same pattern as `tor.FindBinary`) → **Model A**.
  3. Else probe SAM at `sam_address` → **Model B**.
  4. Neither → log WARN, disable, continue (non-fatal, mirrors Tor's
     binary-not-found behavior).
- Backend port: allocate only after a provider is confirmed
  (`net.Listen("tcp", "127.0.0.1:0")`), **plain listener, no
  proxyproto wrapping** (neither i2pd nor SAM prepends a header) —
  serves the same `s.router` handler via its own `http.Server`.
- Model A: `ensureI2PDirs` (`{config_dir}/i2p/` 0700, `{data_dir}/i2p/`
  0700, `{data_dir}/i2p/site/` 0700), regenerate
  `{config_dir}/i2p/tunnels.conf` every startup (parallel to torrc),
  spawn/manage `i2pd` child process, log to `{log_dir}/i2pd.log`.
  Destination key persists at `{data_dir}/i2p/site/site-keys.dat`
  (i2pd's own format — app does not need to parse/convert it, only
  preserve the file).
  `.b32.i2p` address: derive as `base32(sha256(destination))`, no
  padding, lowercased, `+ ".b32.i2p"` (stdlib `crypto/sha256` +
  `encoding/base32` — read destination from i2pd's key file / API,
  per the AI.md reference `b32Address` helper).
- Model B: raw SAMv3 over `net.Conn` to `sam_address` — `HELLO VERSION`,
  load-or-create destination (persisted under
  `{data_dir}/i2p/site/`), `SESSION CREATE STYLE=STREAM`,
  `STREAM FORWARD` → backend port. No new dependency.
- Public API mirrors `tor.Manager` where it makes sense: `Start, Close,
  Running, Address` (returns `.b32.i2p` string), `Monitor`. No
  `RegenerateAddress`/`ApplyKeys`/vanity — not in the I2P spec surface.

## 6. Config schema additions (`src/config/config.go`)

New `I2PConfig` struct under `Server`, matching the AI.md PART 31.2 YAML
block exactly (`server.i2p.*`): `enabled, binary, sam_address,
virtual_port, inbound_length, outbound_length, inbound_quantity,
outbound_quantity, signature_type, bootstrap_timeout`. Add `--i2p` CLI
flag and `I2P_ENABLED` env var override, following the same
flag > env > config > default precedence as every other boolean setting
(`config.ParseBool`/`config.IsTruthy` — never `strconv.ParseBool`).

No changes needed to `TorConfig` — already complete for PART 31.1 (
confirmed in a prior session's survey).

## 7. PART 12 re-verification (no rewrite expected)

`server.go`'s existing Tor Host-header priority-0 FQDN match, no-HTTPS-
upgrade semantics, Tor-variant `/.well-known/security.txt`, and
`tor_privacy_test.go` coverage were confirmed correct and orthogonal to
the ADD_ONION→HiddenServiceDir change. After the `tor.go` rewrite, run
`tor_privacy_test.go` unchanged and confirm it still passes — do not
rewrite it unless it breaks. Extend privacy-rule coverage to `.b32.i2p`
requests per PART 31.2's "trusted for FQDN resolution exactly like
`.onion`" note (same priority-0 treatment, no reverse-proxy header check).

## 8. `.claude/rules/backend-rules.md` regeneration

Confirmed stale: documents only the old ADD_ONION model, zero I2P
coverage. Regenerate the Tor section to match the new torrc/
HiddenServiceDir/PROXY-protocol architecture and the key-migration
requirement; add a new I2P section (opt-in, i2pd/SAM models, plain
loopback backend, no proxyproto). Source of truth is AI.md PART 31.1/31.2
— rules file is a compressed derivative, per the project's own "AI.md is
read-only, rules files are generated from it" convention.

## 9. IDEA.md update

Add an I2P feature-scope entry (currently zero mentions, confirmed via
grep) describing that I2P eepsite support is available and opt-in.

## 10. Test-suite impact

- `src/tor/tor_test.go`, `tor_cov_test.go`, `tor_extended_test.go`
  (~2400 lines combined) — built against ADD_ONION-era behavior, expect
  major rewrite: torrc content assertions (new directives), backend
  listener/proxyproto wrapping, key-migration unit tests (byte-level,
  no real Tor binary needed), `hostname`-file reading instead of
  `AddOnion` response parsing, updated `RegenerateAddress`/`ApplyKeys`
  semantics.
- `src/server/tor_privacy_test.go` — expected to need no changes;
  re-run only, per §7.
- New `src/i2p/i2p_test.go` — from scratch, must hit the project's ≥60%
  coverage gate: provider-resolution logic, tunnels.conf generation,
  SAM protocol framing (can be tested against a mock `net.Conn` without
  a real router), `.b32.i2p` derivation (pure function, easy to test),
  opt-in gating (disabled by default, no port/config/session).
- Unit tests (torrc generation, key migration, `.b32.i2p` derivation, SAM
  framing) run without any real Tor/i2pd binary — pure Go, testable via
  `make test` in Docker. End-to-end tests (real `.onion`/`.b32.i2p`
  bootstrap, the key-migration verification in §2) belong in
  `tests/docker.sh`/`tests/incus.sh` (Phase 2, manual, per
  testing-rules.md) since they need the real `tor`/`i2pd` binaries.
- All builds/tests run in Docker (`casjaysdev/go:latest`) or Incus per
  project rules — never on the host (no Go toolchain here).

## Sequencing

1. `tor.go` rewrite + key-migration logic + its unit tests (§1, §2, §10).
2. `go-proxyproto` dependency + `LICENSE.md` entry (§4).
3. `server.go` single call-site edit (§3, Tor half).
4. Verify existing Tor test suite passes in Docker; add the real-binary
   key-migration verification test (§2) via `tests/docker.sh`.
5. I2P config schema (§6) → I2P package (§5) → I2P tests (§10) →
   `server.go` I2P wiring (§3, I2P half).
6. PART 12 re-verification, extend to `.b32.i2p` (§7).
7. `.claude/rules/backend-rules.md` regeneration (§8), `IDEA.md` update (§9).
8. Full `make test` (coverage gate) + `tests/run_tests.sh` in Docker/Incus.
9. Commit — findings-based work but this is one cohesive feature/migration,
   so per gitcommit_conventions.md's "feature work is opposite, one commit
   for the whole feature" rule: one commit per logical sub-unit (tor.go
   rewrite, I2P package, rules/docs sync) rather than one giant commit,
   but not split finding-by-finding since this isn't an audit.

## Explicitly out of scope for this pass

- The rest of the ~1189-line AI.md diff beyond PART 31 (and PART 12
  re-verification). If the user wants a full 33-part re-audit, that's a
  separate follow-up — will be logged to TODO.AI.md, not silently done
  here.
- Building the `{project_name} tor {status, validate, restart,
  regenerate, vanity start, vanity apply, import-keys}` CLI surface is
  implied by `RegenerateAddress`/`ApplyKeys` existing, but the CLI
  command wiring itself is new feature work, not a regression risk
  (confirmed zero existing callers). Included in this plan's §1/§3 only
  to the extent the underlying `Manager` methods need to work correctly;
  actual CLI flag parsing/subcommand wiring will be added as part of
  this same feature pass rather than deferred, since it's explicitly
  named in the new spec (PART 31.1) and required for the migration to be
  complete, not partial.
