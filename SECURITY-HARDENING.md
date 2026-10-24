# Security hardening in this fork

This fork starts from upstream `4bbb4ac94c5ce287df4eff96e6675798c9f774c4`.
The changes address the eight findings in the 2026-10-25 source audit. They are
security fixes, not a guarantee that all dependencies or Telegram are risk-free.
The Go module remains `github.com/gotd/cli` to keep internal imports compatible;
clone and build from `https://github.com/maxvinters/gotd-cli`.

## Changes

| Finding | Result |
| --- | --- |
| Attachment names could escape the output directory and overwrite files | Server names must be a single safe component. Downloads use an anchored directory and exclusive private file creation. Existing files and symlinks are refused. |
| macOS session secrets appeared in process arguments | `/usr/bin/security` receives one bounded, quoted command through stdin. Secret data is absent from argv. A separate read verifies every write. |
| Separate configurations shared Keychain sessions | Keychain service names include SHA-256 of the complete absolute config path. Old global entries are never automatically adopted or deleted. |
| Account labels could escape session/cache directories | Config loading and commands validate labels before accessing account paths. |
| Existing config/cache/session files could retain open permissions or be truncated | Reads require private regular files. Writes use private temporary files and atomic replacement. Final symlinks and group/world-writable parent directories are refused on Unix. |
| Debug RPC logging exposed authentication and message payloads | Diagnostics contain types, timing and numeric RPC error codes only, on stderr. |
| Remote text could send terminal control sequences | Text output escapes control characters; streaming output and individual fields also escape embedded line breaks. JSON preserves data through JSON escaping. |
| Interactive 2FA echoed the password; argv flags exposed secrets | TTY password input disables echo. Password/token argv flags are rejected. Use prompts or secret-manager-provided environment variables. |

Go and CI use 1.27.3. Network/crypto dependencies are updated. GitHub Actions
are pinned to commit SHAs; GoReleaser is pinned to an exact version. Release
artifacts have checksums and a configured GitHub artifact attestation step.
This change does not publish a release or configure release credentials.

## Before first login

Build from the reviewed fork. Obtain your own Telegram API credentials and
configure them with `tg init`; source builds have no embedded credentials.
Use a separate directory for each configuration, owned by your OS user,
preferably with mode 0700:

```sh
mkdir -p .tg-state
chmod 700 .tg-state
./tg --config .tg-state/config.yaml init --app-id YOUR_ID --app-hash YOUR_HASH
./tg --config .tg-state/config.yaml login
```

Never put a password or bot token literally into a shell command. For automation,
have a secret manager provide `TG_PASSWORD` or `BOT_TOKEN` in the child process
environment. Environment values can still be inspected by sufficiently privileged
processes; keep the OS user account trusted. Proxy credentials and bot tokens in
config files remain secrets. Do not commit config, session or cache files.

## Compatibility and recovery

- Labels contain 1–64 ASCII letters, digits, `_` or `-`. `all` is reserved;
  `default` refers to the top-level account and cannot be a named-map key.
  Rename incompatible labels/config entries before using this fork.
- On Unix, existing config/session/cache files with group/other permission bits
  are refused before reading. After checking the exact paths and ownership,
  use `chmod 600 /absolute/path/to/file`. Use `chmod 700` on your dedicated
  state directory. Fix ownership or ACLs separately if needed. Do not blindly
  change permissions across a shared directory.
- macOS users with only an old global Keychain session must log in again. A
  private legacy file beside the config can be migrated into the new scoped
  Keychain entry. Migration retains that file to avoid deleting another
  process's replacement; after stopping every CLI process, remove the exact
  legacy file yourself if you no longer need it. Explicit logout removes it.
- Legacy files are identified by directory, account label and credential seed.
  Two config files in the same directory can therefore migrate the same old
  session. Use separate state directories and a fresh login for independent
  identities; the new Keychain namespaces separate later updates.
- Moving/renaming the config changes its Keychain namespace and requires a new
  login unless a scoped legacy file exists. Keep the config path stable.
- Keychain sessions use a versioned compressed encoding and a bounded command
  buffer. An unusually large session fails closed with guidance to select
  `keychain: false`; file storage remains private but is not encrypted by tg.
- Downloads never overwrite, including an explicit `--out` file. Choose a new
  name or remove a verified old file first. A failed download retains its
  private partial file and reports the path; it is not a completed attachment.
- Debug output no longer includes full RPC payloads. Remote newlines/tabs appear
  escaped in text fields. Scripts needing original values should consume JSON
  as data and avoid printing decoded untrusted strings directly to a terminal.

## Validation and remaining limits

Run the following with Go 1.27.3:

```sh
go mod verify
go mod tidy -diff
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run --timeout 5m
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Regression tests use synthetic Telegram data and an injected Keychain runner.
Normal tests never read or change the real login Keychain. They cover rejected
paths, exclusive downloads, secret-free diagnostics, config isolation, private
atomic writes, migration failure/replacement, bounded command quoting and
password input. CI runs the full race suite on Linux and macOS.

The updated source scan has no vulnerable imported package or reachable symbol.
The database still reports the unmaintained OpenPGP package at module level;
this CLI does not import it. A clean scan cannot detect unknown vulnerabilities.

No live Telegram account, real Keychain ACL behavior, or release publication was
used to validate these changes. Unix mode checks do not inspect ACLs, directory
ownership or every ancestor. On Windows, protect the state directory with your
user's ACL; Unix mode bits do not enforce Windows access control. The OS account,
its parent directories and configured proxy must be trusted. Atomic writes
prevent partial reads but do not serialize separate CLI processes: concurrent
configuration/cache/session updates remain last-writer-wins. Stop other CLI
processes before changing credentials, migrating or logging out.
