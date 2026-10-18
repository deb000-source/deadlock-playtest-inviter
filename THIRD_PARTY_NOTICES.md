# Third-party acknowledgements

This project uses independent open-source dependencies via Go modules:

- `github.com/paralin/go-steam` — BSD-3-Clause
- `github.com/paralin/go-deadlock` — MIT
- `github.com/skip2/go-qrcode` — MIT
- `github.com/sirupsen/logrus` — MIT
- `golang.org/x/term` — BSD-3-Clause

Their authors retain their respective copyrights. This file is a summary,
not a substitute for the upstream license texts.

`scripts/collect-licenses.ps1` collects the root LICENSE / COPYING files and
available NOTICE / PATENTS files from the resolved modules used by the build,
including indirect dependencies, plus the Go runtime and standard library.
The output is `dist/THIRD_PARTY_LICENSES.txt`. The local build script and
release workflow generate this file and distribute it beside the executable
and the project's `LICENSE.txt`. Regenerate it after dependency updates.

This application is not affiliated with or endorsed by Valve or Steam.
