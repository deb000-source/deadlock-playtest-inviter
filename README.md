# Deadlock Playtest Inviter

Send a Deadlock playtest invitation to a Steam friend without downloading or launching Deadlock. Runs locally on your computer; there is no hosted invitation service.

[Русская инструкция](README_RU.md) · [Downloads](https://github.com/deb000-source/deadlock-playtest-inviter/releases) · [Report a bug](https://github.com/deb000-source/deadlock-playtest-inviter/issues/new/choose)

## Requirements

- Windows 10/11, **64-bit Intel/AMD** (the downloadable Windows amd64 build).
- Your own Steam account with Deadlock access and permission to invite friends.
- The friend's SteamID64 or numeric Steam AccountID / Friend Code.
- Steam mobile app with Steam Guard for QR login.

The tool uses your account's existing invitation capability. Steam friendship requirements, account restrictions and invitation limits still apply. This is an unofficial project, independent of Valve.

## Download and run

1. Open [Releases](https://github.com/deb000-source/deadlock-playtest-inviter/releases) and download `deadlock-inviter-windows-amd64.exe` from the latest release's **Assets**. The **Source code** ZIP is for building the program yourself. If there is no release yet, use [Build from source](#build-from-source).
2. Put the EXE in a folder and open PowerShell in that folder. **Go and Deadlock are not needed** to run the release EXE.
3. Replace `FRIEND_STEAM_ID` below with your friend's numeric Steam ID, then run:

```powershell
.\deadlock-inviter-windows-amd64.exe --auth qr FRIEND_STEAM_ID
```

4. A QR image opens. Scan it in the **official Steam mobile app → Steam Guard**. Check the account and the device name **Deadlock Inviter (QR)** before approving.
5. Wait for the result in PowerShell. The program exits after one invitation request.

`Success` / code `0` means the Game Coordinator accepted the request. Your friend can check [Steam Playtest Invites](https://store.steampowered.com/account/playtestinvites); a successful response alone does not verify delivery or promise an arrival time.

The EXE is not code-signed. Download it from this repository's releases; check its checksum and source before granting access to your Steam account.

### Verify the download

Download `SHA256SUMS.txt` from the same release and compare its EXE entry with:

```powershell
Get-FileHash .\deadlock-inviter-windows-amd64.exe -Algorithm SHA256
```

Matching hashes detect a damaged or different download; they are not a publisher signature. Release assets also include the project license and third-party license texts.

## Login and session options

| Command / option | What it does |
| --- | --- |
| `--auth qr` | Always request a new Steam Guard QR login. |
| `--auth credentials` | Enter your Steam login and password, then complete Steam Guard if requested. Password and Guard-code input are hidden. |
| `--auth auto` | Default: try the saved session, otherwise show a login-method menu. |
| `--auth cache` | Use only the saved session; fail if unavailable. |
| `--username LOGIN` | Set the Steam account login for credentials mode; also check which account owns a cached session. |
| `--cache-session=false` | Do not read or save the session cache for this run. |
| `--logout` | Delete the local cached session and exit; no friend's ID needed. |
| `--version`, `--help` | Show the version or command help. |

**Put flags before the friend's ID.** Examples:

```powershell
# Reuse a saved session, or choose how to log in
.\deadlock-inviter-windows-amd64.exe FRIEND_STEAM_ID

# Log in with a password entered interactively
.\deadlock-inviter-windows-amd64.exe --auth credentials --username YOUR_LOGIN FRIEND_STEAM_ID

# Log in by QR without using persistent storage
.\deadlock-inviter-windows-amd64.exe --auth qr --cache-session=false FRIEND_STEAM_ID

# Remove the saved session
.\deadlock-inviter-windows-amd64.exe --logout
```

On Windows, a token accepted by Steam CM can be cached at `%APPDATA%\deadlock-inviter\session.dpapi`, encrypted with **Windows DPAPI for the current Windows user**. When Steam rejects a cached token in auto mode, the program offers a fresh login. DPAPI does not protect against malicious software running as that user.

`--logout` deletes the local file. It **does not revoke** the session on Valve's servers. Use Steam's account security settings to revoke an authorized session when needed.

## Optional configuration

No configuration file is required. If you want saved defaults, copy [.env.example](.env.example) to `.env` in the folder from which you run the command:

```dotenv
AUTH_METHOD=auto
STEAM_USERNAME=
CACHE_SESSION=true
```

Priority: command-line flags → existing environment variables → `.env` → defaults. `STEAM_PASSWORD` is supported as an environment variable or `.env` value, but storing it in a plaintext file is discouraged; leave it unset to use hidden console input. Do not store Steam Guard codes. Leave the optional undocumented `DEADLOCK_LOCATION` setting empty unless you specifically need it.

Never upload `.env`, tokens, session files or active login QR images. See [Security](SECURITY.md) for private vulnerability reports.

## Results and troubleshooting

| Code | Meaning |
| --- | --- |
| `0 Success` | GC accepted the invitation request. Ask your friend to check Steam. |
| `3 InvalidFriend` | The target is not an eligible friend. |
| `4 NotFriendsLongEnough` | Steam's friendship-duration requirement is not met. |
| `5 AlreadyHasGame` | The friend already has access. |
| `6 LimitedUser` | Account restrictions apply. |
| `7 InviteLimitReached` | The invitation limit has been reached. |

Other codes are printed as errors. Valve can change the interface and responses.

- **The window closes immediately:** run the command from PowerShell instead of double-clicking the EXE.
- **QR does not open:** use the local image path shown in the terminal. Do not share the image. A refreshed QR replaces the old one.
- **Login expires:** start again and complete Steam Guard within the authentication timeout (five minutes).
- **Steam / GC times out:** check your connection and try again later. If a request may already have been sent, check with your friend before retrying.
- **Saved login fails:** try `--auth qr` or remove the local cache with `--logout`.

## Build from source

Install [Go 1.27.2 or newer](https://go.dev/dl/), download the source or clone this repository, and open PowerShell in its folder:

```powershell
go mod download
go mod verify
go test -mod=readonly ./...
.\build.ps1
```

The local Windows build is `dist\deadlock-inviter.exe`. The script also prepares license files and `SHA256SUMS.txt` under `dist`. Dependencies are recorded in `go.mod` and `go.sum`; ordinary builds use read-only module mode. Update dependencies deliberately and review both files together.

Source builds on Linux/macOS do not have a persistent session cache. Packaged releases target Windows amd64; Linux/macOS runtime support is not part of the release checks.

## Contributing and releases

See [CONTRIBUTING.md](CONTRIBUTING.md) for changes, checks and reports, and [CHANGELOG.md](CHANGELOG.md) for planned release notes. Changes go through a Pull Request. CI tests on Windows/Linux and builds the Windows executable. A separate release job attaches the built executable, checksums and license files when a `v*` tag is pushed.

The program uses an unofficial Steam / Deadlock GC interface, which may change. MIT covers the original project code; dependencies retain their own licenses. See [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
