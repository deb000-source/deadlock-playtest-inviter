# Security policy

This is an **unofficial** Steam GC client. It processes sensitive Steam session
credentials: its source, dependencies and binary should be treated as security
sensitive. The project does not promise compatibility or Valve support.

## Recommendations

- Prefer QR authentication and verify device details inside the official Steam app.
- Never publish `.env`, your Steam password, Steam Guard codes, access tokens,
  DPAPI blobs, or live QR screenshots.
- The DPAPI cache is tied to the Windows user profile; DPAPI cannot defend
  against malware running with the same account's permissions.
- For a higher-assurance workflow, build the executable yourself and inspect
  pinned Go dependencies. Do not trust binaries shared by unknown parties.
- Use Steam security settings to revoke previously approved sessions if a
  refresh token is exposed. Local `--logout` only deletes the cached file.
- There is no anonymous telemetry, local HTTP server, or embedded analytics.

## Reporting vulnerabilities

Use [GitHub private vulnerability reporting](https://github.com/deb000-source/deadlock-playtest-inviter/security/advisories/new)
to report a suspected vulnerability. Include reproduction steps with test data.
Do not include real passwords, tokens, Steam Guard codes, session caches, or live
QR images, and do not post leaked credentials in a public issue.
