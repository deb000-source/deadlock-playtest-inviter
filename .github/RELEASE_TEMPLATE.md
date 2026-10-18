## Deadlock Playtest Inviter

Send a playtest invitation to an eligible Steam friend without installing or launching Deadlock. Use your own Steam account with existing Deadlock access and permission to invite.

### Download

- **deadlock-inviter-windows-amd64.exe** — Windows 10/11, Intel/AMD 64-bit; Go is not required.
- **SHA256SUMS.txt** — checksum for the executable.
- **LICENSE.txt** and **THIRD_PARTY_LICENSES.txt** — project and dependency license texts.

Open PowerShell in the download folder, replace `FRIEND_STEAM_ID` with your friend's numeric Steam ID, and run:

```powershell
.\deadlock-inviter-windows-amd64.exe --auth qr FRIEND_STEAM_ID
```

Approve the QR login in the official Steam mobile app. Steam's invitation limits and eligibility requirements still apply. A successful GC response means the request was accepted; ask your friend to check Steam for delivery.

[English instructions](https://github.com/deb000-source/deadlock-playtest-inviter#readme) · [Инструкция на русском](https://github.com/deb000-source/deadlock-playtest-inviter/blob/main/README_RU.md)

### По-русски

Скачай EXE, открой PowerShell в его папке и подставь Steam ID друга:

```powershell
.\deadlock-inviter-windows-amd64.exe --auth qr STEAM_ID_ДРУГА
```

Отсканируй QR через Steam Guard и подтверди вход. Go и установленный Deadlock не нужны. Аккаунт отправителя уже должен иметь доступ к игре и право приглашать друзей.

The executable is unsigned. This is an unofficial project and is not affiliated with Valve. Do not share passwords, tokens, session files or active login QR images.
