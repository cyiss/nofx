# Security review — 2026-09-06

## Source remediation — 2026-09-07

User authorized fixing the findings in security-review.zh-CN.md. Keep work in the downloaded checkout, preserve audit evidence, no live trading or deployment, no unrelated refactors.

Design: fail closed at existing risk/ownership boundaries; invalidate sessions durably on password changes; reuse payment authorization or stop on ambiguous payment status; explicit opt-in telemetry; build the local reviewed source with secret-free context and loopback defaults. Retain normal fees and user-owned wallet export. Prefer these targeted fixes over disabling entire exchanges or replacing authentication architecture.

- [x] A. Authentication/API executor: regression tests then fix private batch/public config authorization, proxy trust, durable token revocation, ownership updates, first-registration race, orphan adoption and wallet environment persistence (api/auth/store/cli only).
- [x] B. Trading/payment executor: fault-injection regressions then fail closed for partial XYZ queries and leverage failures; prohibit fresh payment nonces on ambiguous repeated 402; run affected tests and benchmarks (trader/mcp/payment only).
- [x] C. Configuration/privacy executor: regression tests then reject placeholder secrets, opt-in/minimize telemetry, update examples and relevant tests (config/crypto/telemetry/.env.example only).
- [x] D. Deployment/dependencies: remove unneeded unsafe TA-Lib build if unused, otherwise pin verified HTTPS input; protect .env permissions and Docker context; loopback ports; install/build local source instead of silently selecting upstream images; update applicable dependencies/toolchain with official sources.
- [x] E. Integration verification: full safe unit/build checks where possible, targeted regression red/green evidence, behavioral diff and benchmarks, shell/compose validation; fix regressions.
- [x] F. Independent risk-gate/spec/code review, address findings, document remaining limitations and exact verification results.

Validation commands: targeted `go test` per affected package (mocked HTTP only), then safe package suite after reviewing tests for live endpoints; `go build ./...`; affected benchmarks; `npm ci --ignore-scripts`, frontend tests/build; `bash -n` installer scripts; Docker Compose config rendering without starting services. Record environment blockers accurately rather than counting them as passes.

Scope: clone https://github.com/cyiss/nofx and assess the checked-out source without starting trading services, running installer scripts, or supplying credentials.

- [x] Obtain local checkout and record revision (dev, 638d4042118995fbf1a38d3822b1139aa3c6b467; shallow history).
- [x] Review install/build/deployment and dependency supply chain.
- [x] Review authentication, authorization, and exposed API routes.
- [x] Review private keys, outbound data, payments, and trading execution risks.
- [x] Validate findings independently and check dependency advisories without lifecycle scripts.
- [x] Write Chinese security report with evidence, severity, mitigations, and limitations.

Original audit scope: no application source changes planned. Behavior/performance benchmark and live execution tests are not applicable to this read-only security review; any checks performed and checks not performed will be documented.

Results: security-review.zh-CN.md records 8 prioritized findings plus conditional configuration/history risks. Three prioritized high-severity issues concern HTTP build inputs and XYZ trading risk checks. No conclusive backdoor evidence in reviewed paths; not suitable for immediate real-money/public deployment. Only audit files were added.

Verification: Git connectivity/source diff passed; isolated telemetry behavior test passed with in-memory HTTP interception; OSV and npm official advisory results saved. npm audit CLI did not complete and was stopped; not counted as a passing check. Independent report verifier approved subject to citation/length corrections, all applied and checked. See verification/results.txt and lessons.md.

Remediation completed: source changes and regression tests implemented, Go full build and affected native-CGO suites passed, frontend 160 tests and production build passed. Independent risk gate approved after fixing review findings. See security-remediation.zh-CN.md and verification/remediation-results.txt. No deploy, live trading, commit or push performed.

## Local Docker deployment — 2026-09-07
- [x] Move Docker image storage to D: with rollback copy and verify engine.
- [x] Prepare D: runtime configuration/source snapshot with private generated secrets.
- [x] Build and start local containers, verify health/UI and persistent mount location.
- [x] Independent startup review and document commands/locations.
