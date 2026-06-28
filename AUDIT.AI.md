# AUDIT.AI.md — Full Spec Compliance Audit

Total violations: 85
Critical: 5 | High: 30 | Medium: 40 | Low: 10

## All Violations

### [HIGH] PART 2: LICENSE & ATTRIBUTION
**File:** /root/Projects/github/apimgr/pastebin/LICENSE.md
**Issue:** Multiple go.mod dependencies are absent from the Third-Party Licenses table. Missing direct dependency: github.com/redis/go-redis/v9 (BSD-2-Clause). Missing indirect dependencies: github.com/charmbracelet/bubbletea, github.com/charmbracelet/bubbles, github.com/charmbracelet/lipgloss, github.com/charmbracelet/colorprofile, github.com/charmbracelet/x/ansi, github.com/charmbracelet/x/cellbuf, github.com/charmbracelet/x/term, github.com/atotto/clipboard, github.com/aymanbagabas/go-osc52/v2, github.com/clipperhouse/displaywidth, github.com/clipperhouse/stringish, github.com/clipperhouse/uax29/v2, github.com/dgryski/go-rendezvous, github.com/erikgeiser/coninput, github.com/lucasb-eyer/go-colorful, github.com/mattn/go-localereader, github.com/mattn/go-runewidth, github.com/muesli/ansi, github.com/muesli/cancelreader, github.com/muesli/termenv, github.com/rivo/uniseg, github.com/xo/terminfo. Spec (PART 2) states 'All third-party dependencies MUST have their licenses attributed in LICENSE.md'.
**Fix:** Add all missing dependencies to the '## Third-Party Licenses' table in LICENSE.md with their version, license (BSD-2-Clause for redis; MIT for most charmbracelet/muesli packages; BSD-3-Clause for atotto/clipboard), and copyright holder. Run 'go-licenses csv ./...' inside casjaysdev/go:latest to generate the full list.

### [MEDIUM] PART 2: LICENSE & ATTRIBUTION
**File:** /root/Projects/github/apimgr/pastebin/README.md
**Issue:** README.md has no license badge. Spec (PART 2) states 'Every README.md MUST include a license badge' and that it should appear 'in the badges section near the top of README.md'. The file currently has only a '## License' text section near line 167 with no badge anywhere.
**Fix:** Add '[![License](https://img.shields.io/github/license/apimgr/pastebin)](LICENSE.md)' in a badges section near the top of README.md (immediately after the title or description).

### [LOW] PART 3: PROJECT STRUCTURE
**File:** /root/Projects/github/apimgr/pastebin/.dockerignore
**Issue:** .dockerignore is missing required exclusion entries. Spec lists as MUST include: 'Makefile', '.idea/', '.vscode/', '*.swp', '*.swo' under the IDE/Build files categories. Additionally 'Jenkinsfile' (which exists at the project root) and '.gitea/', '.forgejo/', '.gitlab-ci.yml' (CI/CD entries) are absent.
**Fix:** Add the following lines to .dockerignore: 'Makefile', 'Jenkinsfile', '.idea/', '.vscode/', '*.swp', '*.swo', '.gitea/', '.forgejo/', '.gitlab-ci.yml'.

### [MEDIUM] PART 5
**File:** /root/Projects/github/apimgr/pastebin/src/config/config.go
**Issue:** Boolean parsing functions (ParseBool, MustParseBool, IsTruthy, IsFalsy) are embedded in config.go instead of a separate src/config/bool.go file. The spec explicitly names this file at AI.md lines 891, 3701, 4589, and 7410 as a required file in the project structure.
**Fix:** Extract ParseBool, MustParseBool, IsTruthy, IsFalsy, truthyValues, and falsyValues from config.go into a new file src/config/bool.go with package config.

### [LOW] PART 5
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** No auto-migration from server.yaml to server.yml on startup. The spec states: 'If server.yaml found, auto-migrate to server.yml on startup.' The code at line 833 only constructs the path as server.yml and never checks for a server.yaml file.
**Fix:** Before loading cfgFile, check whether a server.yaml exists in configDir. If found and server.yml does not exist, rename/copy it to server.yml and log the migration. Add this check in the config load section of run() (around line 833 of src/main.go).

### [MEDIUM] PART 6
**File:** /root/Projects/github/apimgr/pastebin/src/mode/mode.go
**Issue:** The function SetAppMode() required by the spec (AI.md line 9009) does not exist. The code has Set(mode string) error instead. The spec requires the exact name SetAppMode(m string) with no error return.
**Fix:** Rename Set() to SetAppMode() (or add SetAppMode as an alias) and change the signature to match the spec: func SetAppMode(m string). Update all callers (src/main.go calls mode.Initialize which internally calls Set).

### [MEDIUM] PART 6
**File:** /root/Projects/github/apimgr/pastebin/src/mode/mode.go
**Issue:** The function SetDebugEnabled(enabled bool) required by the spec (AI.md line 9020) does not exist. The code has SetDebug(enabled bool) instead.
**Fix:** Rename SetDebug() to SetDebugEnabled() (or add SetDebugEnabled as an alias). Update all callers in src/main.go (line 753: mode.SetDebug(true)).

### [MEDIUM] PART 6
**File:** /root/Projects/github/apimgr/pastebin/src/mode/mode.go
**Issue:** The function IsDebugEnabled() bool required by the spec (AI.md line 9053) does not exist. The code has IsDebug() bool instead.
**Fix:** Rename IsDebug() to IsDebugEnabled() (or add IsDebugEnabled as an alias). Update all callers across the codebase.

### [HIGH] PART 6
**File:** /root/Projects/github/apimgr/pastebin/src/mode/mode.go
**Issue:** The FromEnv() function required by the spec (AI.md line 9068) does not exist. The code has Initialize(cliMode string) error instead, which only reads the MODE env var but does NOT read the DEBUG env var using config.IsTruthy(os.Getenv("DEBUG")).
**Fix:** Add a FromEnv() function to src/mode/mode.go that reads MODE env var via Set() and reads DEBUG env var via config.IsTruthy(os.Getenv("DEBUG")) → SetDebugEnabled(true). The spec implementation at AI.md line 9068 must be followed exactly.

### [HIGH] PART 6
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** The DEBUG environment variable is never read anywhere in the codebase. The spec requires debug priority: (1) --debug CLI flag, (2) DEBUG env var (truthy values), (3) default false. Only the --debug flag is checked (line 752). The DEBUG env var is entirely ignored.
**Fix:** After calling mode.Initialize() in src/main.go (around line 748), add a call to mode.FromEnv() (once that function is added to mode.go) so that DEBUG=true/yes/1/etc. enables debug mode. Alternatively, read os.Getenv("DEBUG") directly and call mode.SetDebugEnabled(config.IsTruthy(os.Getenv("DEBUG"))).

### [CRITICAL] PART 5
**File:** /root/Projects/github/apimgr/pastebin/src/
**Issue:** privilege_unix.go and privilege_windows.go are completely absent from the codebase. The spec (AI.md lines 7749-7872) requires platform-split files implementing isElevated(), canEscalate(), and execElevated() for both Unix and Windows. None of these functions exist anywhere in the source tree.
**Fix:** Create src/service/privilege_unix.go (build tag //go:build !windows) implementing isElevated() via os.Geteuid()==0, canEscalate() via sudo -n check and group membership, and execElevated() via exec.Command("sudo", ...). Create src/service/privilege_windows.go (build tag //go:build windows) implementing the same interface using golang.org/x/sys/windows. Wire isElevated() into service install/uninstall flows and privileged port binding.

### [HIGH] PART 8: Short Flags Constraint
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** The `normalizeArgs` function (line 1061) converts any single-dash multi-character flag to double-dash, e.g. `-debug` → `--debug`, `-port` → `--port`. This makes every long flag also work with a single dash. Spec mandates short flags are ONLY `-h` (help) and `-v` (version); all other flags must be long-form only.
**Fix:** Remove the default conversion case `if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 { out = append(out, "-"+a) }`. Only the explicit `-h` → `--help` and `-v` → `--version` cases should be handled. Any other single-dash flag should be passed through unchanged (and will fail flag parsing with an appropriate error).

### [MEDIUM] PART 7/8: Runtime Detection — os.Hostname()
**File:** /root/Projects/github/apimgr/pastebin/src
**Issue:** No call to `os.Hostname()` exists anywhere in the server source (grep across all of `src/` confirms zero occurrences outside of test files). AI.md §Runtime Detection Rules (line 461) requires `os.Hostname()` at startup for hostname/FQDN detection. Hardcoding or omitting is explicitly forbidden.
**Fix:** Call `os.Hostname()` during server startup (Phase 5, step 6 in the startup sequence) to obtain the machine hostname. Use this value in the startup banner, status output, and wherever the spec surfaces hostname information.

### [MEDIUM] PART 7/8: Runtime Detection — runtime.NumCPU()
**File:** /root/Projects/github/apimgr/pastebin/src
**Issue:** No call to `runtime.NumCPU()` exists anywhere in the server source. AI.md §Runtime Detection Rules (line 463) and §Performance Optimization Rules (lines 524, 538) require `runtime.NumCPU()` to size worker pools and concurrency limits at runtime. Without it, the server cannot scale its workers to available CPU resources.
**Fix:** Use `runtime.NumCPU()` at startup to determine the worker pool size (with a minimum of 2 and any user config override). Apply this to any goroutine pool or concurrency-limited operation in the server.

### [MEDIUM] PART 7: TERM=dumb Handling — NewSpinner / ShowProgress
**File:** /root/Projects/github/apimgr/pastebin/src/common/display
**Issue:** The display package contains only `detect.go`, `detect_unix.go`, and `detect_windows.go`. The spec (AI.md lines 9378–9395) explicitly defines and requires `NewSpinner(env *DisplayEnv, message string) Spinner` and `ShowProgress(env *DisplayEnv, percent int)`, along with `TextSpinner` and `ANSISpinner` types, to provide graceful TERM=dumb fallback. All binaries must handle TERM=dumb per spec. These functions are completely absent.
**Fix:** Implement `NewSpinner`, `ShowProgress`, `TextSpinner`, and `ANSISpinner` in `src/common/display/spinner.go` (or similar). `NewSpinner` must return a `TextSpinner` (plain text) when `env.IsDumbTerminal()` is true and an `ANSISpinner` otherwise. `ShowProgress` must print `N% complete\n` in dumb mode and an ANSI progress bar otherwise.

### [LOW] PART 7: Terminal Package Module Structure — resize.go and symbols.go
**File:** /root/Projects/github/apimgr/pastebin/src/common/terminal
**Issue:** The terminal package contains only `size.go`. The spec module structure (AI.md) also requires `resize.go` for SIGWINCH terminal resize handling and `symbols.go` for Unicode/ASCII symbol definitions. Both files are absent.
**Fix:** Create `src/common/terminal/resize.go` with a SIGWINCH signal handler that updates terminal size on window resize (Unix build tag; no-op on Windows). Create `src/common/terminal/symbols.go` with Unicode box-drawing characters and their ASCII fallbacks, selecting the appropriate set based on terminal capabilities.

### [HIGH] PART 11 (Security & CSRF)
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** CSRF middleware is fully configured (CSRFConfig with cookie_name='csrf_token', HeaderName='X-CSRF-Token', csrf_token_secret stored in DB) but never implemented. The middleware chain (lines 465–488) does not include any CSRF token issuance or validation middleware. The `Sec-Fetch-Site` check in `secFetchMiddleware` is a partial defense but does not fulfill the spec-required double-submit cookie pattern.
**Fix:** Add a `csrfMiddleware` to the chi router that: (1) generates an HMAC-signed token from `csrf_token_secret` and sets it as a `SameSite=Strict; Secure; HttpOnly` cookie, (2) on POST/PUT/PATCH/DELETE with cookie auth, validates the `X-CSRF-Token` header or form field matches the cookie via `crypto/subtle.ConstantTimeCompare`. Exempt paths in `cfg.Web.CSRF.ExemptPaths` and Bearer-token requests. Add it to `r.Use()` after `secFetchMiddleware`.

### [HIGH] PART 11 (Security & CSRF)
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/create.html
**Issue:** The POST form (line 41: `action="/create" method="POST"`) has no CSRF hidden input field. The spec requires state-mutating HTML forms to carry a CSRF token.
**Fix:** Add `<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">` inside the form, and pass `CSRFToken` from the CSRF middleware into the template data map in the handler.

### [HIGH] PART 11 (Security & CSRF)
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/remove.html
**Issue:** The POST form (line 57: `action="/remove/{{.ID}}" method="POST"`) has no CSRF hidden input field.
**Fix:** Add `<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">` inside the form and pass `CSRFToken` from the CSRF middleware into the template data in `handleRemove`.

### [MEDIUM] PART 9 (Error Handling) / PART 11 (Security)
**File:** /root/Projects/github/apimgr/pastebin/src/server/security_middleware.go
**Issue:** Two `http.Error()` calls return `Content-Type: text/plain` instead of the canonical JSON error envelope required by the spec. Line 50 (path traversal block) returns plain `"bad request"` and line 225 (blocklist rejection) returns plain `"forbidden"`.
**Fix:** Replace both `http.Error()` calls with a `writeJSON()`-style helper that sets `Content-Type: application/json` and writes `{"ok":false,"error":"BAD_REQUEST","message":"..."}` and `{"ok":false,"error":"FORBIDDEN","message":"..."}` respectively.

### [MEDIUM] PART 9 (Error Handling)
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** Multiple `http.Error()` calls return `Content-Type: text/plain` instead of the canonical JSON error envelope: (1) line 1190 inside `writeJSON` fallback sends a JSON string via `http.Error()` which sets wrong Content-Type; (2) line 1605 returns plain text 'internal server error' on paste fetch failure; (3) line 1677 returns plain text 'bad request' in `handleRemoveSubmit`; (4) lines 2126 and 2139 in `renderTemplate` return plain text on template errors.
**Fix:** For API/JSON contexts, replace `http.Error()` with a call to `writeJSON(w, status, map[string]interface{}{"ok":false,"error":"ERROR_CODE","message":"..."})`. For the `writeJSON` fallback (line 1190), manually write the JSON with the correct Content-Type header. For HTML route errors (lines 1605, 1677, 2126, 2139), render an error template or redirect rather than returning plain text.

### [LOW] PART 9 (Error Handling)
**File:** /root/Projects/github/apimgr/pastebin/src/graphql/graphql.go
**Issue:** Line 38 (`http.Error(w, "method not allowed", http.StatusMethodNotAllowed)`) and line 79 (`http.Error(w, "json encode error", http.StatusInternalServerError)`) return plain text with `Content-Type: text/plain`. Even for GraphQL endpoints, 405 and 500 errors should use a structured response.
**Fix:** Return GraphQL-spec error objects: `writeJSON(w, 405, map[string]interface{}{"errors":[{"message":"method not allowed"}]})` and `writeJSON(w, 500, map[string]interface{}{"errors":[{"message":"internal server error"}]})` to maintain consistent `Content-Type: application/json`.

### [HIGH] PART 13
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** HealthResponse struct contains a `Cluster ClusterInfo` field (line 95) that is not present in the PART 13 canonical HealthResponse specification. The spec defines a fixed field order with no ClusterInfo entry.
**Fix:** Remove the `Cluster ClusterInfo` field and its `ClusterInfo` type definition (lines 114–118), and remove its population in `buildHealthResponse()` (line 1278). The canonical struct ends at `Stats StatsInfo`.

### [HIGH] PART 13
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** `buildTorInfo()` sets `status = "running"` when Tor is active (around line 1310). PART 13 specifies the only valid values for `TorInfo.Status` are `"healthy"`, `"starting"`, and `"error"`. The value `"running"` is not a valid spec value.
**Fix:** Change `status = "running"` to `status = "healthy"` inside `buildTorInfo()` so it conforms to the three allowed values: `"healthy"` (running), `"starting"` (not yet running), `"error"` (failed).

### [CRITICAL] PART 20
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** The `/metrics` route (lines 509–516) is registered with only bearer token auth (inside `metrics.Collector.Handler()`). PART 20 requires the endpoint to be internal-only via **both** firewall/IP restriction **and** optional bearer token. No IP restriction middleware wraps the metrics handler.
**Fix:** Wrap the metrics handler registration with an IP-allowlist middleware that rejects requests from non-loopback, non-private IPs. Add an `AllowedIPs []string` field to `MetricsConfig` (config.go) and enforce it in the middleware before delegating to the bearer-token check.

### [HIGH] PART 20
**File:** /root/Projects/github/apimgr/pastebin/src/config/config.go
**Issue:** `MetricsConfig` (lines 162–167) has no IP-allowlist field. The struct only has `Enabled`, `Endpoint`, and `Token`. PART 20 requires firewall/IP restriction support, which requires a configurable IP allowlist in the config schema.
**Fix:** Add `AllowedIPs []string \`yaml:"allowed_ips"\`` to `MetricsConfig` and use it in the metrics route middleware to restrict access to loopback and any explicitly listed IPs/CIDRs.

### [HIGH] PART 15
**File:** /root/Projects/github/apimgr/pastebin/src/config/config.go
**Issue:** The TLS configuration struct `TLSConfig` is mapped to the YAML key `ssl` (line 62: `yaml:"ssl"`), making the config path `server.ssl.*`. PART 15 explicitly requires `server.tls.*` keys in the config file, and the api-rules note states "TLS config: `server.tls.*` keys in config (NOT `--ssl-*` CLI flags)".
**Fix:** Change the yaml tag on the `TLS TLSConfig` field from `yaml:"ssl"` to `yaml:"tls"` in `ServerConfig`. Update any server.yml templates, defaults, and documentation to use `server.tls.*` instead of `server.ssl.*`. Add backward-compat migration to auto-rename `.yaml:"ssl"` → `tls` on first load.

### [HIGH] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/css/main.css
**Issue:** Required long-string CSS utility classes are missing: .long-string, .ip-address, .onion-address, .api-token, .hash, .uuid, .monospace-data — none defined anywhere in main.css. Spec mandates these with word-break:break-all; overflow-wrap:break-word; font-family:monospace applied to IPv6, Tor .onion, API tokens, hashes, UUIDs, and Base64 content.
**Fix:** Add all required classes to main.css: .long-string, .ip-address, .onion-address, .api-token, .hash, .uuid, .monospace-data { word-break: break-all; overflow-wrap: break-word; font-family: monospace; }

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/css/main.css
**Issue:** Desktop breakpoint uses @media (min-width: 1025px) but spec requires @media (min-width: 1024px). Off-by-one causes 1024px-wide viewports to miss desktop styles.
**Fix:** Change all occurrences of @media (min-width: 1025px) to @media (min-width: 1024px) in main.css.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/css/main.css
**Issue:** Dracula-inspired color palette not implemented. Spec requires --bg: #1e1e2e, --fg: #cdd6f4, --accent: #89b4fa as the dark theme base. Current values are --color-bg-primary: #0a0a0f and --color-accent-primary: #6366f1, which diverge from the required palette.
**Fix:** Update dark theme CSS custom properties to use the Dracula-inspired values: --bg: #1e1e2e, --fg: #cdd6f4, --accent: #89b4fa (and adjust derived color tokens accordingly).

### [HIGH] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/css/main.css
**Issue:** No toast notification system CSS defined. Spec requires #toast-container and .toast rules. The showToast() function is also absent from main.js. All toast styling and positioning is missing.
**Fix:** Add #toast-container and .toast CSS rules to main.css, and implement showToast(message, type) in main.js to create and append .toast elements to #toast-container.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/css/main.css
**Issue:** No @media (prefers-reduced-motion: reduce) rule is present anywhere in main.css. Spec requires honoring this media query by disabling/reducing CSS transitions and animations for users who prefer reduced motion (WCAG 2.1 AA requirement).
**Fix:** Add @media (prefers-reduced-motion: reduce) block to main.css that sets transition: none and animation: none on all animated elements.

### [HIGH] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/js/main.js
**Issue:** showToast() function is not defined in main.js. Spec prohibits JavaScript alert() calls (none found — compliant) and requires a toast notification system via showToast() instead. Without this function, any code attempting to call showToast() will throw a ReferenceError.
**Fix:** Implement showToast(message, type='info') in main.js that creates a .toast element, appends it to #toast-container, and auto-removes it after a timeout.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/js/main.js
**Issue:** No offline detection logic present in main.js. Spec requires: navigator.onLine check on load, window.addEventListener('online') and window.addEventListener('offline') event handlers, and toggling a visible #offline-indicator element. Templates also lack the #offline-indicator element.
**Fix:** Add offline/online event listeners to main.js that show/hide a #offline-indicator element. Add <div id="offline-indicator" hidden> to all base templates.

### [LOW] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/static/js/main.js
**Issue:** showUpdateBanner() function at lines 70-77 sets styles via banner.style.cssText (inline CSS via JavaScript). Spec prohibits setting inline styles anywhere — CSS classes must control all visual state.
**Fix:** Remove the style.cssText assignment and instead add/remove a CSS class (e.g. .update-banner--visible) that is defined in main.css with the appropriate display and positioning rules.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/home.html
**Issue:** Inline <script> block at lines 172-180 defines the copyCode() function directly in the template. Spec requires all JavaScript logic to be in external files (/static/js/main.js); inline script blocks with logic are forbidden.
**Fix:** Move copyCode() to /static/js/main.js, attach it via data attributes (e.g. data-copy-block), and remove the inline <script> block from home.html.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/paste.html
**Issue:** Inline <script> block at lines 80-98 defines copyToClipboard() function directly in the template. Spec requires all JavaScript logic in external files only.
**Fix:** Move copyToClipboard() to main.js (or consolidate with the existing [data-copy] handler), wire up via data attribute on the button, and remove the inline <script> block.

### [HIGH] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/recent.html
**Issue:** The recent pastes page renders NO content server-side. The paste list, empty state, and pagination are all hidden with style="display:none" and populated entirely via an async JS fetch to /api/v1/pastes. This violates the progressive enhancement requirement: the page shows only a spinner and nothing else without JavaScript.
**Fix:** Server-render the paste list in the /recent handler by passing a Pastes slice to the template. Render <div class="paste-item"> elements server-side for all pastes. JavaScript can optionally enhance with client-side pagination but must not be required for core content display.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/recent.html
**Issue:** Inline <script> block (lines 81-156) contains the entire page logic: loadPastes(), escapeHtml(), formatDate(), and the initial loadPastes(1) call. This is business logic in an inline script, which spec forbids.
**Fix:** After implementing server-side rendering for the paste list, move any remaining JS enhancement to main.js and remove the inline <script> block from recent.html.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/about.html
**Issue:** Multiple inline style= attributes present (e.g. style="padding:var(--space-6);" style="margin-top:var(--space-6);"). Spec prohibits inline CSS in templates.
**Fix:** Replace all inline style= attributes with named CSS utility classes defined in main.css.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/help.html
**Issue:** 21 inline style= attributes present throughout the template. Spec prohibits any inline CSS in templates.
**Fix:** Extract all inline styles to named classes in main.css and apply those classes to the elements.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/contact.html
**Issue:** Multiple inline style= attributes present in contact.html. Spec prohibits inline CSS in templates.
**Fix:** Replace all inline style= attributes with CSS classes defined in main.css.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/qr.html
**Issue:** Multiple inline style= attributes in qr.html AND an inline <script> block that generates the QR code URL via an external API (api.qrserver.com). Logic in inline scripts is forbidden; external QR API usage may also violate spec requirement to generate QR server-side.
**Fix:** Move QR generation to the server handler (use a Go QR library, return an embedded image or data URI), remove the inline <script> block, and replace all inline style= attributes with CSS classes.

### [LOW] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/emb.html
**Issue:** Inline style="margin-left:auto;" attribute present. Spec prohibits inline CSS in templates.
**Fix:** Add a utility class (e.g. .ml-auto { margin-left: auto; }) to main.css and apply it to the element.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** pageData() (line 2162) only passes SiteTitle and Theme. The /server/about page requires tagline, description, features list, and links sourced from IDEA.md project variables. handleAbout calls s.renderTemplate(w, r, "about.html", s.pageData()) with no additional data, so about.html cannot render dynamic content from IDEA.md.
**Fix:** Define an AboutPageData struct with Tagline, Description, Features []string, and Links fields. Populate it from config or embedded IDEA.md values in handleAbout, and pass it to the template. Update about.html to reference these fields instead of hardcoded text.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** PWA icon handlers serve SVG content at .png URLs with Content-Type: image/svg+xml. Spec requires PNG format for PWA icons. Additionally only 3 sizes are provided (180, 192, 512) vs. the 9 required sizes (72, 96, 128, 144, 152, 180, 192, 384, 512) plus a maskable variant.
**Fix:** Generate or embed actual PNG icon files for all required sizes (72, 96, 128, 144, 152, 180, 192, 384, 512 + maskable). Serve them with Content-Type: image/png. Register routes for all sizes. Update manifest.json icons array to list all sizes with correct type and purpose fields.

### [LOW] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/server.go
**Issue:** Manifest theme_color in handleManifest is "#238636" while all HTML template meta theme-color tags use "#6366f1". These must match; both must also align with the required Dracula-inspired accent color #89b4fa.
**Fix:** Set a single canonical accent/theme color constant. Use it in handleManifest's theme_color field and in the <meta name="theme-color"> tag in all templates. Update to #89b4fa per spec.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/home.html
**Issue:** No "Skip to content" link at the start of <body>. Spec requires a skip link for WCAG 2.1 AA keyboard navigation compliance. All templates are missing this.
**Fix:** Add <a href="#main-content" class="skip-link">Skip to content</a> as the first element inside <body> in all templates. Add corresponding CSS for .skip-link that is visually hidden by default and visible on focus.

### [MEDIUM] PART 16
**File:** /root/Projects/github/apimgr/pastebin/src/server/templates/home.html
**Issue:** No theme toggle button in any template header. Spec requires a theme toggle button (dark/light/auto cycling) visible in the header, with state persisted in localStorage. The toggleTheme() function exists in main.js but no button triggers it.
**Fix:** Add a theme toggle <button> element to the <nav> in all templates' header. The button should call toggleTheme() and display the current mode (dark/light/auto). Apply minimum 44x44px touch target sizing.

### [MEDIUM] PART 17 (Email)
**File:** /root/Projects/github/apimgr/pastebin/src/common/email/email.go
**Issue:** SMTP auto-detection priority order is wrong. The spec (PART 17 table) requires: 4={fqdn}, 5={global_ipv4}, 6=mail.{fqdn}, 7=smtp.{fqdn}. The code appends globalIP (lines 129-135) before fqdn and its subdomains (lines 136-145), reversing priorities 4 and 5.
**Fix:** In AutoDetect(), move the globalIP candidates block to after the bare fqdn block. Order should be: 127.0.0.1 → 172.17.0.1 → gatewayIP → fqdn → globalIP → mail.fqdn → smtp.fqdn.

### [MEDIUM] PART 20 (Metrics)
**File:** /root/Projects/github/apimgr/pastebin/src/metrics/metrics.go
**Issue:** pastebin_scheduler_tasks_running is defined as a plain Gauge (no labels) at line 192, but the spec (PART 20 Scheduler Metrics table) requires it to be a GaugeVec with a 'task' label so per-task running counts are observable. The scheduler.go callers at lines 250/252 use Inc()/Dec() without a task label.
**Fix:** Change SchedulerTasksRunning to promauto.NewGaugeVec with []string{"task"} label. Update scheduler/scheduler.go execute() to call SchedulerTasksRunning.WithLabelValues(e.id).Inc() and .Dec() instead of the plain .Inc()/.Dec().

### [MEDIUM] PART 20 (Metrics)
**File:** /root/Projects/github/apimgr/pastebin/src/metrics/metrics.go
**Issue:** pastebin_cache_bytes Gauge (with 'cache' label) is absent. The spec (PART 20 Cache Metrics table and the reference implementation) requires a cache_bytes GaugeVec reporting current cache size in bytes. Only cache_size (item count) is defined.
**Fix:** Add: CacheBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "cache_bytes", Help: "Current cache size in bytes."}, []string{"cache"})

### [LOW] PART 20 (Metrics)
**File:** /root/Projects/github/apimgr/pastebin/src/metrics/metrics.go
**Issue:** pastebin_go_gc_pause_total_seconds Counter is absent. The spec (PART 20 Go Runtime Metrics table) requires this counter: 'Total time spent in GC pauses'. The collectRuntime() function populates goroutines, alloc, sys, and gc_runs_total but never sets gc_pause_total_seconds.
**Fix:** Add: GoGCPauseTotalSeconds = promauto.NewCounter(prometheus.CounterOpts{Namespace: ns, Name: "go_gc_pause_total_seconds", Help: "Total time spent in GC pauses."}) and in collectRuntime() add: GoGCPauseTotalSeconds.Add(float64(ms.PauseTotalNs) / 1e9) (note: use a stateful lastPauseNs to compute the delta each scrape, since runtime.MemStats.PauseTotalNs is cumulative).

### [LOW] PART 19 (GeoIP)
**File:** /root/Projects/github/apimgr/pastebin/src/geoip/geoip.go
**Issue:** When EnableWHOIS is true the code reuses asn.mmdb and country.mmdb but never downloads or stores a dedicated whois.mmdb file. The spec (PART 19 Database Sources table) shows whois.mmdb as a distinct file (URL: cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb). The Info struct also has no RegistrantOrg field despite the spec listing 'registrant_org' as a WHOIS field.
**Fix:** Add urlWHOIS constant (same URL as urlCountry). When EnableWHOIS is true, download it to {dir}/whois.mmdb and open a separate whoisDB reader. Add RegistrantOrg string field to Info and CityRecord/CountryRecord-style WHOISRecord struct with maxminddb:"registrant_org" tag. Populate Info.RegistrantOrg from the whois reader in Lookup().

### [MEDIUM] PART 21
**File:** /root/Projects/github/apimgr/pastebin/src/maintenance/maintenance.go
**Issue:** The `Manifest` struct is missing the `created_by` field. The spec's manifest.json example (line 28043-28057) shows `"created_by": "operator"` as a required field, but the `Manifest` struct only has `Version`, `CreatedAt`, `AppVersion`, `Contents`, `Encrypted`, `EncryptionMethod`, and `Checksum`. The field is never populated in `Backup()`.
**Fix:** Add `CreatedBy string \`json:\"created_by\"\`` to the `Manifest` struct and populate it in `Backup()` with the caller's identity (e.g., `"operator"` or the OS username via `os.Getenv("USER")`).

### [HIGH] PART 21
**File:** /root/Projects/github/apimgr/pastebin/src/maintenance/maintenance.go
**Issue:** `VerifyBackup()` (lines 315-403) never validates the manifest's `Checksum` field against the actual archive bytes. The spec (lines 28219-28222 and 28493-28494) lists `Checksum valid: SHA-256 matches manifest` as a Fatal check in both post-creation verification and restore verification. The code decodes the manifest into a local variable `m` but `m.Checksum` is never used or compared.
**Fix:** Before walking the tar entries, compute SHA-256 of the decrypted archive bytes (`data`) and compare against `m.Checksum` (expected format `sha256:<hex>`). Because the manifest is embedded in the archive, a two-pass approach is needed: first extract the manifest checksum in a pass, then verify the hash of `data` against it, aborting if they differ.

### [HIGH] PART 21
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** `--maintenance restore` always passes an empty password string to `maintenance.Restore()` (line 330: `maintenance.Restore(maintenanceArg, mcConfigDir, mcDataDir, "")`). The spec (lines 28439-28450) requires CLI to interactively prompt `Enter backup password:` when the backup file is encrypted (`.tar.gz.enc`). Instead the code passes `""`, causing `VerifyBackup` to immediately error `"verify: encrypted backup requires password"` with no way for the user to provide a password.
**Fix:** Detect when `maintenanceArg` ends in `.enc` and interactively read a password from stdin (using `term.ReadPassword` or equivalent that suppresses echo) before calling `maintenance.Restore()`. Pass the collected password as the fourth argument.

### [HIGH] PART 21
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** There is no `--password` flag and no interactive password prompt for `--maintenance backup`. The spec (line 28147) states `"The --password flag is required if encryption is enabled"` and the example flow shows the CLI prompting for a password before creating an encrypted backup. `BackupOptions.Password` is always set to `""` (empty string) in the `"backup"` case (lines 313-323), so encrypted backups cannot be created via `--maintenance backup`.
**Fix:** Either add a `--password` flag to the CLI that is captured into a variable and passed into `BackupOptions.Password`, or check if encryption is configured (once a backup config section is added) and interactively prompt for the password before calling `maintenance.Backup()`.

### [MEDIUM] PART 22
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** `--update branch <name>` only prints `"Update branch set to: {branch}"` (lines 434-436) and explicitly comments that it is informational only. It does not write the branch preference to `server.yml` or any config store. The spec (line 28527) defines `branch {stable|beta|daily}` as `"Set update branch"` — an action that persists the choice.
**Fix:** Load the server config, set the update branch field (requires adding a config field), write back to `server.yml`, and confirm to the user.

### [MEDIUM] PART 22
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** Both `--update check` (line 389) and `--update yes` (line 405) hardcode `"stable"` as the branch argument to `updater.CheckForUpdate()`. They never read the configured branch from the config file, so `--update branch beta` followed by `--update yes` will still check the stable channel instead of the beta channel.
**Fix:** Load the server config before the update block, read the configured update branch (once the config field exists), and pass it to `CheckForUpdate()` instead of the hardcoded `"stable"` string.

### [HIGH] PART 21
**File:** /root/Projects/github/apimgr/pastebin/src/config/config.go
**Issue:** There is no `server.backup.*` configuration section in `config.go`. The spec (lines 28087-28096, 28179-28188) requires `server.backup.encryption.enabled`, `server.backup.compliance.enabled`, and `server.backup.retention.{max_backups,keep_weekly,keep_monthly,keep_yearly}` as configurable keys in `server.yml`. These fields are completely absent from the config struct. `SchedulerTask.Retention int` only covers a single integer and is not the correct location for this spec-required section.
**Fix:** Add a `BackupConfig` struct with nested `BackupEncryptionConfig` (field `Enabled bool`), `BackupComplianceConfig` (field `Enabled bool`), and `BackupRetentionConfig` (fields `MaxBackups`, `KeepWeekly`, `KeepMonthly`, `KeepYearly int`) structs. Wire the struct into the main `ServerConfig` as `Backup BackupConfig \`yaml:\"backup\"\``.

### [HIGH] PART 23
**File:** /root/Projects/github/apimgr/pastebin/src/service/winsvc_other.go
**Issue:** Required functions `isElevated()`, `canEscalate()`, and `execElevated()` are completely absent. The spec (PART 23 / service-rules.md) requires all three in platform-split `_unix.go`/`_windows.go` files. The code only implements `isPrivileged()`, which only checks current UID but cannot prompt for escalation via sudo/pkexec/doas.
**Fix:** Add platform-split files (e.g., `src/service/privilege_unix.go` and `src/service/privilege_windows.go`) implementing `isElevated() bool`, `canEscalate() bool` (checks sudoers/wheel/admin group), and `execElevated() error` (re-execs the current process under sudo/runas/pkexec). The existing `isPrivileged()` should be renamed to `isElevated()` and the missing functions added.

### [HIGH] PART 24
**File:** /root/Projects/github/apimgr/pastebin/src/service/service.go
**Issue:** `NoNewPrivileges=yes` is missing from the generated systemd unit in `installSystemd()` (line ~211-236). The spec (PART 24, service-rules.md) explicitly lists `NoNewPrivileges=yes` as a required hardening directive alongside `ProtectSystem=strict` and `PrivateTmp=yes`.
**Fix:** Add `NoNewPrivileges=yes` to the `[Service]` section of the `serviceContent` string template in `installSystemd()`, after the existing `PrivateTmp=yes` line.

### [CRITICAL] PART 24
**File:** /root/Projects/github/apimgr/pastebin/src/service/service.go
**Issue:** OpenRC and SysVinit service managers are completely missing. `DetectServiceManager()` only detects systemd, runit, launchd, Windows, and BSD rc.d. The `ServiceType` enum has no `ServiceOpenRC` or `ServiceSysV` values. PART 24 states "ALL projects MUST have built-in service support for ALL service managers" and explicitly documents OpenRC (Alpine/Gentoo/Devuan) and SysVinit (legacy Linux/init.d) with full templates and detection logic (`/sbin/openrc-run` absent + `systemctl` absent + `/etc/init.d/` present with `update-rc.d` or `chkconfig`).
**Fix:** Add `ServiceOpenRC` and `ServiceSysV` to the `ServiceType` enum. Extend `DetectServiceManager()` to check for `/sbin/openrc-run` (OpenRC) before falling back to SysV detection. Implement `installOpenRC()`, `uninstallOpenRC()`, `installSysV()`, `uninstallSysV()` per the spec templates, and add cases for them in all switch statements (Install, Uninstall, Start, Stop, Restart, Reload, Disable).

### [HIGH] PART 23
**File:** /root/Projects/github/apimgr/pastebin/src/service/service.go
**Issue:** Service user UID/GID is hardcoded to `serviceUID = 300` (line 20). The spec requires a `findAvailableSystemID()` function that dynamically scans from 899 down to 200, skips a defined `reservedIDs` map (999-980, 101-110, 170-179), checks both `/etc/passwd` and `/etc/group` for availability, and errors if no ID is free. The hardcoded value does not verify availability or check the reserved list at install time.
**Fix:** Remove the `serviceUID = 300` constant. Implement `findAvailableSystemID() (int, error)` per the spec (scan 899→200, skip `reservedIDs`, call `user.LookupId` and `user.LookupGroupId`). Call it in `installSystemd()` (and other Unix install paths) to obtain the UID/GID at install time.

### [HIGH] PART 23
**File:** /root/Projects/github/apimgr/pastebin/src/service/service.go
**Issue:** Service user home directory is set to `/nonexistent` (line 243) in the `useradd` call inside `installSystemd()`. The spec (PART 23, System User Requirements table) requires the home directory to be the config directory (`/etc/{project_org}/{internal_name}` → `/etc/apimgr/pastebin`) or data directory (`/var/lib/apimgr/pastebin`).
**Fix:** Change the `-d` argument in the `useradd` command from `/nonexistent` to `fmt.Sprintf("/etc/%s/%s", orgName, appName)` (i.e., `/etc/apimgr/pastebin`).

### [MEDIUM] PART 24
**File:** /root/Projects/github/apimgr/pastebin/src/service/service.go
**Issue:** Windows service installation uses `sc.exe` CLI (lines 465-479) instead of the `golang.org/x/sys/windows/svc/mgr` Go package. The spec (PART 24) shows an explicit Go implementation using `mgr.Connect()` and `m.CreateService()` with `mgr.Config{StartType: mgr.StartAutomatic, ServiceStartName: ""}` (empty = Virtual Service Account). `installWindows()` is also not in a `_windows.go` build-tag file, meaning it compiles on all platforms.
**Fix:** Move `installWindows()` and `uninstallWindows()` into `src/service/winsvc_windows.go` (guarded by `//go:build windows`) and implement them using `mgr.Connect()`, `m.CreateService()`, and `s.Delete()` from `golang.org/x/sys/windows/svc/mgr` per the spec's Go implementation snippet.

### [CRITICAL] PART 27 (CI/CD)
**File:** /root/Projects/github/apimgr/pastebin/.github/workflows/daily.yml
**Issue:** The `build` job (lines 24-101) has no `container: image: casjaysdev/go:latest` block and instead uses `actions/setup-go` (line 51-53). The spec explicitly states: "All Go jobs use `container: image: casjaysdev/go:latest` — tools pre-installed; never install inline" and "DO NOT use `actions/setup-go` — the container already has the correct Go version."
**Fix:** Remove the `actions/setup-go` step and add `container:\n  image: casjaysdev/go:latest` to the `build` job, matching the spec's daily.yml example at AI.md line 32226-32228.

### [CRITICAL] PART 27 (CI/CD)
**File:** /root/Projects/github/apimgr/pastebin/.github/workflows/beta.yml
**Issue:** The `build` job (lines 20-97) has no `container: image: casjaysdev/go:latest` block and instead uses `actions/setup-go` (lines 46-50). The spec explicitly states: "All Go jobs use `container: image: casjaysdev/go:latest`" and "DO NOT use `actions/setup-go`".
**Fix:** Remove the `actions/setup-go` step and add `container:\n  image: casjaysdev/go:latest` to the `build` job, matching the spec's beta.yml example at AI.md lines 32083-32085.

### [HIGH] PART 25 (Makefile)
**File:** /root/Projects/github/apimgr/pastebin/Makefile
**Issue:** The `build` target's `go build` commands (lines 65, 74, 81, 89) are all missing the explicit `-buildvcs=false` flag. The spec (AI.md lines 30155-30167) requires `-buildvcs=false` inline on every `go build` call in the `build` target. The same omission applies in the `local` target (lines 109, 114) and the `dev` target (lines 203-205).
**Fix:** Add `-buildvcs=false` to every `go build` invocation: `go build -buildvcs=false -trimpath -ldflags ...`. For `dev`, the spec pattern is `go build -buildvcs=false -o $$BUILD_DIR/$(PROJECTNAME) ./src`.

### [MEDIUM] PART 27 (CI/CD)
**File:** /root/Projects/github/apimgr/pastebin/.github/workflows/ci.yml
**Issue:** The `ci.yml` is missing the three post-build jobs required by the spec's job-order description in cicd-rules.md: `coverage` (run after `build`), `image-scan` (run after `build`), and `upload-artifacts` (run after `build`). The spec states: "Then: `coverage`, `image-scan`, `upload-artifacts` (need: build)".
**Fix:** Add `coverage`, `image-scan`, and `upload-artifacts` jobs that `needs: [build]` and run with `if: github.event_name != 'schedule'`. These should follow the same pattern as the parallel security jobs already present.

### [MEDIUM] PART 30
**File:** /root/Projects/github/apimgr/pastebin/src/common/i18n/locales/ar.json
**Issue:** ar.json has 19 extra keys not present in en.json, violating the spec requirement that all 7 locale files MUST have identical keys. The extra keys are Arabic-specific plural forms: plurals.days.few/many/two/zero, plurals.hours.few/many/two, plurals.items.few/many/two, plurals.minutes.few/many/two, plurals.results.few/many/two, plurals.users.few/many/two.
**Fix:** Add the 19 missing plural keys (plurals.days.few, plurals.days.many, plurals.days.two, plurals.days.zero, plurals.hours.few, plurals.hours.many, plurals.hours.two, plurals.items.few, plurals.items.many, plurals.items.two, plurals.minutes.few, plurals.minutes.many, plurals.minutes.two, plurals.results.few, plurals.results.many, plurals.results.two, plurals.users.few, plurals.users.many, plurals.users.two) to en.json (and all other locale files) so that every locale file has identical key sets. English values can use the 'other' form as the common fallback.

### [MEDIUM] PART 30
**File:** /root/Projects/github/apimgr/pastebin/src/common/i18n/i18n_test.go
**Issue:** No build-time key parity test exists. The spec requires a build-time check that ensures all languages have the same keys as en.json. The i18n_test.go has TestIsSupported, TestTranslate, TestDirection, etc., but no TestKeyParity (or equivalent) that iterates all locale files and asserts their key sets are identical.
**Fix:** Add a TestKeyParity function to i18n_test.go that loads all locale files, flattens their keys, and asserts that every locale's key set equals en.json's key set exactly (no missing keys, no extra keys).

### [MEDIUM] PART 28 / PART 30
**File:** /root/Projects/github/apimgr/pastebin/src/main.go
**Issue:** The --color flag is documented and exposed as {auto|yes|no} (line 1125 in printHelp: '--color {auto|yes|no}') but the spec (binary-rules.md and testing-rules.md) requires the values to be {auto|always|never}. The client binary (src/client/main.go line 288 and 917) also uses 'auto, yes, no'. While the implementation tolerates 'always'/'never' as internal aliases, the public CLI interface does not match the spec.
**Fix:** Change the primary flag values to {auto|always|never} in both src/main.go (printHelp text and colorFlag definition) and src/client/main.go (colorFlag definition at line 288, help text at line 917, and shell completions in src/shell/shell.go). Keep yes/no as silent backward-compatible aliases if desired, but the canonical documented values must be always/never.

### [MEDIUM] PART 29
**File:** /root/Projects/github/apimgr/pastebin/docs/stylesheets/
**Issue:** docs/stylesheets/light.css is missing. The spec (PART 29) defines both docs/stylesheets/dark.css and docs/stylesheets/light.css as theme CSS files and provides a full template for light.css. Only dark.css exists. Additionally, mkdocs.yml only references dark.css in extra_css (line 47) but not light.css, diverging from the spec template which includes both.
**Fix:** Create docs/stylesheets/light.css using the Light Theme CSS template from the PART 29 spec, and add '- stylesheets/light.css' to the extra_css list in mkdocs.yml (after the dark.css entry).

### [LOW] PART 28
**File:** /root/Projects/github/apimgr/pastebin/tests/run_tests.sh
**Issue:** tests/run_tests.sh, tests/docker.sh, and tests/incus.sh are all missing the required '# @@License : WTFPL' header. The spec states: 'All shell scripts are WTFPL — # @@License : WTFPL in header'.
**Fix:** Add '# @@License : WTFPL' as a comment line in the header section of all three test scripts: tests/run_tests.sh, tests/docker.sh, and tests/incus.sh.

### [HIGH] PART 31 — Tor Hidden Service
**File:** /root/Projects/github/apimgr/pastebin/src/tor/tor.go
**Issue:** `writeIfChanged()` is called to write the torrc file on every startup (line 154). It overwrites the file whenever the generated content differs from what is on disk, destroying any manual operator edits. The spec's `ensureTorrc` pattern requires create-only-if-absent semantics: write the file only if it does not already exist; `updateTorrc` is the separate function for operator-driven config changes.
**Fix:** Replace the `writeIfChanged(torrcPath, ...)` call in the startup path with a create-if-absent guard: `if _, err := os.Stat(torrcPath); os.IsNotExist(err) { os.WriteFile(torrcPath, ...) }`. Add a separate `updateTorrc(path string, cfg Config) error` function (called only when the operator changes Tor settings via API/config reload) that uses `writeIfChanged` or direct write.

### [MEDIUM] PART 31 — Tor Hidden Service
**File:** /root/Projects/github/apimgr/pastebin/src/tor/tor.go
**Issue:** `ensureTorDirs()` calls `os.MkdirAll` and `os.Chmod` but never calls `os.Chown`. The spec requires ownership enforcement on all Tor directories (non-fatal on Windows) so that directories created by a privileged process are owned by the `pastebin` system user after privilege drop.
**Fix:** After each `os.Chmod(d, 0o700)` call inside `ensureTorDirs`, add `if runtime.GOOS != "windows" { _ = os.Chown(d, uid, gid) }` where `uid`/`gid` are the pastebin system user's IDs resolved at startup (non-fatal if chown fails).

### [HIGH] PART 31 — Tor Hidden Service
**File:** /root/Projects/github/apimgr/pastebin/src/tor/tor.go
**Issue:** The `Manager` struct is missing four required lifecycle methods: `Restart() error`, `UpdateConfig(cfg Config) error`, `RegenerateAddress() (string, error)`, and `ApplyKeys(privateKey []byte) (string, error)`. These are mandatory for operator CLI commands (`--maintenance tor restart`, address regeneration, and key provisioning).
**Fix:** Implement the four missing methods on `*Manager`. `Restart` must call `Close` then re-run `Start`. `UpdateConfig` must update the persisted config and call `updateTorrc` then `Restart`. `RegenerateAddress` must delete the existing key material and restart to get a new hidden service address. `ApplyKeys` must write the provided private key bytes to the data directory and restart.

### [HIGH] PART 32 — Client Binary
**File:** /root/Projects/github/apimgr/pastebin/src/client/main.go
**Issue:** `cliConfig` struct (lines 56–66) only contains `Server`, `Update.{Auto,CheckInterval,Channel}`, and `Display.Mode`. The spec's full `cli.yml` schema requires seven additional top-level sections: `auth` (token, token_file), `output` (format, color, pager, quiet, verbose), `tui` (enabled, theme, mouse, unicode), `logging` (level, file, max_size, max_files), `cache` (enabled, ttl, max_size), `debug` bool, and `defaults` (lang, public, expire, syntax, output, limit). All missing fields mean those settings cannot be stored or read from `cli.yml`.
**Fix:** Expand `cliConfig` to match the complete spec schema with all seven sections and their sub-fields. Update `loadCLIConfig` to set the correct defaults for the new fields and plumb the loaded values into the corresponding flag defaults and runtime behaviour.

### [HIGH] PART 32 — Client Binary
**File:** /root/Projects/github/apimgr/pastebin/src/client/main.go
**Issue:** Exit codes 2 (config error), 3 (connection/network error), 4 (auth error), 5 (not-found), and 64 (usage error) are never used. Every error path calls `log.Fatal` or `os.Exit(1)`, making it impossible for scripts and automation to distinguish error categories. The spec mandates these exact exit codes.
**Fix:** Replace all `log.Fatal` / `os.Exit(1)` calls with `os.Exit(N)` where N matches the error category: config/parse failures → 2; network/connection failures → 3; 401/403 responses → 4; 404 responses → 5; invalid flag usage → 64. Define named constants (e.g. `exitConfig = 2`) to avoid magic numbers.

### [MEDIUM] PART 32 — Client Binary
**File:** /root/Projects/github/apimgr/pastebin/src/client/main.go
**Issue:** Line 430 error message reads `"try: create, get, delete, list, update, tui, completions"`. This implies `tui` and `completions` are positional subcommands, but the spec explicitly states there is no `tui` command (TUI auto-launches when no arguments are provided) and `completions` is handled via `--shell completions`, not as a positional argument. The misleading message will confuse users who type `pastebin-cli tui` or `pastebin-cli completions` and get another unknown-command error.
**Fix:** Remove `tui` and `completions` from the error hint string. Rewrite to: `"try: create, get, delete, list, update"` and add a separate line explaining `--shell completions` for shell completion setup. Alternatively hint `"run '%s --help' for usage"` to avoid enumerating commands in an error message.

### [HIGH] PART 32 — Client Binary
**File:** /root/Projects/github/apimgr/pastebin/src/client/main.go
**Issue:** No `EnsureDirs()` call exists at CLI startup. The spec requires that on every invocation the CLI creates its config, data, cache, and log directories (0700 permissions) before any file operations. Currently, only the config file's parent directory is created inside `saveCLIConfig`, which is only called when saving — meaning a fresh install with no prior config has no directories until the user explicitly saves something.
**Fix:** Add an `EnsureDirs()` function that calls `os.MkdirAll` with 0700 on the four user-scope directories (config, data, cache, log). Call it unconditionally at the top of `main()`, before `loadCLIConfig`.

### [HIGH] PART 32 — Client Binary
**File:** /root/Projects/github/apimgr/pastebin/src/client/main.go
**Issue:** `cliConfigPath()` delegates to `paths.GetConfigDir(projectName)` for non-Windows (line 76). `GetConfigDir` returns `/etc/apimgr/pastebin/` when the process runs as root (see `/root/Projects/github/apimgr/pastebin/src/paths/paths.go` line 48). The spec mandates that the CLI binary always uses user-scope directories regardless of privilege level (`~/.config/apimgr/pastebin/` on Linux, `~/Library/Application Support/...` on macOS) and must never use system directories.
**Fix:** In `cliConfigPath()`, replace the `paths.GetConfigDir(projectName)` call with a user-home-based path that bypasses the root check: `home, _ := os.UserHomeDir(); return filepath.Join(home, ".config", "apimgr", projectName, "cli.yml")` (with platform branching for macOS and Windows). Never call the server's `paths` package from CLI code.

### [MEDIUM] PART 32 — Client Binary
**File:** /root/Projects/github/apimgr/pastebin/src/client/main.go
**Issue:** `cmdDelete` constructs the request URL as `"/api/v1/pastes/"+id` (line 616) without percent-encoding the paste ID in the path segment. If a paste ID ever contains characters that are invalid or reserved in URL paths (e.g. `/`, `#`, `?`), the request will be malformed or target a different resource.
**Fix:** Wrap the paste ID with `url.PathEscape(id)`: `c.url("/api/v1/pastes/"+url.PathEscape(id)+"?token="+url.QueryEscape(token))`. Apply the same fix to any other route that concatenates a user-supplied ID directly into a URL path (e.g. `cmdGet`).
