# Contributing

Bug reports, documentation fixes and code changes are welcome. English and Russian are both fine.

## Report a problem

Use the [issue templates](https://github.com/deb000-source/deadlock-playtest-inviter/issues/new/choose). Include the program version, Windows version, login method, exact error and steps to reproduce. Replace real Steam IDs and account names with placeholders in shared logs. Never include passwords, tokens, Steam Guard codes, session files or active QR codes.

Report vulnerabilities privately using [SECURITY.md](SECURITY.md).

## Make a change

1. Fork the repository if you do not have write access.
2. Create a branch for one focused change.
3. Use Go 1.27.2 or newer and run the checks below.
4. Open a Pull Request explaining the problem, the resulting behavior and your validation.

```powershell
go mod download
go mod verify
go test -mod=readonly ./...
go vet -mod=readonly ./...
```

Format changed Go files with `gofmt`. Test changes to login with mock responses; automated tests must not log in to real Steam accounts or send invitations. Keep secrets out of tests and fixtures.

For a Windows build, run `.\build.ps1`. The script also prepares the license and checksum files in `dist`.

## Dependencies and workflows

Keep `go.mod` and `go.sum` together. Run `go mod tidy` when intentionally changing dependencies and inspect the diff. Refresh third-party license files with `scripts/collect-licenses.ps1` after changing dependencies. Ordinary builds use read-only module mode.

Pin GitHub Actions to full commit SHAs. Test and build jobs should have read-only repository permissions. Only the separate release job needs `contents: write`; never run untrusted PR code with that permission.

## Review and merge

`main` is protected: use a Pull Request, resolve review conversations, and use Squash merge. The solo maintainer can merge without a second person's approval. New code should include tests for the behavior it changes. Follow the [code of conduct](CODE_OF_CONDUCT.md).

## Prepare a release

Review [CHANGELOG.md](CHANGELOG.md), update the version in `config.go` when needed, and prepare the description using [.github/RELEASE_TEMPLATE.md](.github/RELEASE_TEMPLATE.md). Check the Windows build and its license/checksum files. Real Steam tests are manual and require the account owner's consent.

Publishing source and pushing a version tag are separate actions. A pushed `v*` tag triggers the release workflow, which publishes the generated files. Do not push a release tag until the maintainer has approved that release.
