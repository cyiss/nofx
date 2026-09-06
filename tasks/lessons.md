# Review verification notes — 2026-09-06

- Use checked-out source line numbers, not web-rendered line counts; web renderers may collapse blank lines.
- Measure literal lengths from the file rather than counting mentally. The sample JWT value exceeds the 32-byte minimum, but this does not make a public sample a safe signing key.
- Trace setters to actual readers before claiming a configuration edit affects trading; an unused prompt field is not proof of live order influence.
- Separate historical/orphan-database conditions from the current normal account-reset path.
- Distinguish dependency version matches from reachable application vulnerabilities and static findings from exercised tests.

## Remediation review lessons — 2026-09-07

- Check retries across callers: a payment helper alone cannot prevent a provider or market fallback from issuing a fresh authorization.
- Error-wrapping defer must update the actual named return value; inner short declarations can shadow err. Assert errors.Is directly in fault tests.
- A successful SDK call may hide an exchange error envelope. Validate acknowledgement before cancellation of existing protective orders.
- Separate native-driver evidence from temporary dependency overlays; do not claim Docker runtime or PostgreSQL integration from static validation.

- Browser telemetry must be audited separately from backend opt-in settings; HTML tag-manager scripts bypass server flags. Removed unconditional GTM from self-hosted entry HTML before local launch.

- Snapshot copy exclusions must target absolute runtime paths: excluding every directory named data also removes web/src/data source fixtures. Verify tracked source completeness before Docker build.
