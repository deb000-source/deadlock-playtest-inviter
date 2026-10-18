# Changelog

## [0.2.0] - 2026-10-18

### Features

- Send one Deadlock playtest invitation to an existing Steam friend from a local CLI.
- QR login through the official Steam mobile app.
- Credentials login with Steam Guard codes or mobile/email approval in the same session.
- Optional Windows DPAPI session cache, cache-only login and local logout.
- English and Russian usage instructions.
- Windows amd64 executable, checksums and license files prepared by the release workflow.

### Reliability and security

- Wait for credential approval until the authentication deadline, without a fixed ten-poll limit.
- Explicit command-line cache settings override environment defaults.
- Reject authentication redirects and invalid RSA keys; do not send private agreement URLs to logs.
- Track dependencies with `go.mod` and `go.sum` and use read-only module mode for builds.
- Require Go 1.27.2 or newer for the standard library security fixes.
- Use pinned GitHub Actions, read-only build permissions and a separate release job.

First public release.
