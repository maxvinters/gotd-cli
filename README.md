# tg — Telegram CLI for humans and agents

## Hardened fork

This fork is maintained at [maxvinters/gotd-cli](https://github.com/maxvinters/gotd-cli).
Security changes and compatibility notes are documented in [SECURITY-HARDENING.md](SECURITY-HARDENING.md).
Build this fork from its checkout with Go 1.27.3 or newer; the upstream module path is retained for internal imports.

```sh
git clone https://github.com/maxvinters/gotd-cli.git
cd gotd-cli
go build -trimpath -o tg ./cmd/tg
```

Use your own Telegram application credentials for source builds. No live account is needed for the default test suite.

`tg` is a single static Go binary for driving a Telegram **personal account** (or a
bot) from the command line or an AI agent: message yourself, list and triage chats,
read history, send/forward/edit/delete messages, upload and download files, manage
groups and channels, and stream new messages in real time.

Built on [`gotd/td`](https://github.com/gotd/td).

## Installation

Build from source using the commands above. This fork has no published hardened
release yet; upstream binaries do not contain these fixes.

## Quick start

```console
$ tg init --app-id APP_ID --app-hash APP_HASH       # write config
$ tg login                                         # QR login — scan once from a logged-in device
$ tg whoami                                         # confirm you're authenticated
$ tg send "Hello world"                             # message yourself (Saved Messages)
$ tg send --peer @gotd_test "Hello world"           # message a peer
```

App credentials are required. Get your own from
<https://my.telegram.org> and pass them to `tg init --app-id APP_ID --app-hash APP_HASH`.
Source builds contain no embedded credentials.

The config is written to the `gotd` subdirectory of your config dir, e.g.
`~/.config/gotd/gotd.cli.yaml`. The session persists too, so subsequent commands
run headless. On macOS the session is stored in the login Keychain by default; an
existing file session is migrated into the Keychain automatically on first use.
Set `keychain: false` in the config to keep it in a file alongside the config
instead (useful for headless macOS). Other platforms always use a file.

### Login options

```console
$ tg login                     # QR (default): scan from Settings → Devices → Link Desktop Device
$ tg login --phone +1234567890  # phone-code login (use --phone= to be prompted for the number)
$ tg login                     # or supply TG_PASSWORD through your secret manager
```

For optional bot login, supply `BOT_TOKEN` through your secret manager before
running `tg init`, then pass `--bot` to commands that support it (e.g. `tg whoami --bot`).
Password and bot-token flags are rejected to keep secrets out of process arguments.

## Agent-friendly by design

- **Structured output:** `--output json` (`-o json`) on any command emits a stable
  envelope `{"schema":1,"data":…}` on stdout. Logs, prompts and progress go to stderr.
- **Consistent peers:** every `<peer>` accepts `me`/`self`, `@username`, a phone number,
  or a `t.me/…` link. Resolved access-hashes are cached locally.
- **Safety:** destructive actions (`delete`, `delete-history`, `unpin-all`) require `--yes`.
- **Shell completion:** `tg completion bash|zsh|fish|powershell`, with dynamic completion
  for peers, accounts, output format and enum flags.

```console
$ tg chats list --output json | jq '.data.chats[].peer.username'
$ tg history @durov --limit 20 -o json
```

### Agent skill

This repo ships an installable [Claude Code](https://claude.com/claude-code) skill
(`skills/tg`) that teaches an agent how to drive Telegram with `tg` — setup checks,
the JSON/peers conventions, the `--yes` safety gate, and common recipes. Install it
with [`skills`](https://www.npmjs.com/package/skills):

```console
$ npx skills add https://github.com/maxvinters/gotd-cli --all
# or a single skill:
$ npx skills add https://github.com/maxvinters/gotd-cli --skill tg
```

## What you can do

Run `tg --help` for the full, grouped command list, or `tg <command> --help` for details.

- **Messaging:** `send`, `reply`, `edit`, `delete`, `delete-history`, `forward`,
  `history`, `search` (`--global`), `read`, `pin`/`unpin`/`unpin-all`/`pinned`,
  `react`/`unreact`/`reactions`, `draft`/`drafts`, `schedule`, `poll`, `link`, `context`.
- **Media:** `upload` (with `--type` video/audio/voice/gif/sticker), `album`, `download`,
  `stickers`.
- **Chats & contacts:** `chats list`, `chat get`/`full`, `mute`/`unmute`,
  `archive`/`unarchive`, `resolve`, `search-public`, `subscribe`, `contacts` (list,
  search, add, delete, block/unblock, blocked, import).
- **Groups & channels:** `create-group`, `create-channel`, `invite`, `leave`,
  `participants`/`admins`/`banned`, `promote`/`demote`, `ban`/`unban`, `slow-mode`,
  `set-title`/`set-about`/`set-photo`, `invite-link`/`join-link`, `topics`,
  `recent-actions`.
- **Profile & folders:** `profile` (update, set/delete photo, status), `folders`
  (list, create, add-chat/remove-chat, delete, reorder).
- **Realtime:** `watch [peer]` streams new messages as JSON lines; `wait [peer]` blocks
  until the next message (with `--timeout`).

## Global flags

| Flag | Description |
| --- | --- |
| `-o, --output text\|json` | output format (default `text`) |
| `-a, --account <label>` | select an account, or `all` to fan out across accounts |
| `-c, --config <path>` | config file to use |
| `--proxy <url>` | `socks5://…` or `tg://proxy?…` (MTProxy) |
| `--test` | connect to the Telegram test server (persisted by `tg init --test`) |

## Multiple accounts

Add named accounts and select them per command:

```console
$ tg accounts add work --app-id … --app-hash …
$ tg login --account work
$ tg accounts                       # list accounts + auth status
$ tg chats list --account work
$ tg watch --account all            # stream every account concurrently, labeled
```

## Using the test server

Initialize a config against the Telegram **test server**, then log in with a test
number. Test mode only switches the DC list — it uses the same app credentials as
production (so the same app-id/app-hash requirement applies):

```console
$ tg init --app-id APP_ID --app-hash APP_HASH --test # persists test: true
$ tg login --phone 9996621234   # test number for DC 2; the code is 22222
$ tg whoami
```
