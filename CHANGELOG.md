# Changelog

## Unreleased

### Security

- Reject remote filenames that can escape a download directory and refuse to overwrite existing files or symbolic links.
- Isolate macOS Keychain sessions by configuration and keep session material out of process arguments.
- Validate account labels before using them in session and cache paths.
- Write private configuration, cache and file-session data atomically with mode 0600.
- Hide interactive 2FA passwords and reject password/token command-line arguments.
- Escape terminal controls in text results and keep debug RPC payloads out of logs.
- Update vulnerable module versions and build with Go 1.27.3; pin workflow actions and release tools, and attest future release artifacts.

### Compatibility

- Existing globally named Keychain sessions are not automatically adopted. Log in again for each configuration after upgrading.
- Downloads no longer overwrite an existing destination, including an explicitly selected output file.
- Account labels must be safe names rather than paths; reserved labels cannot be used for named accounts.
- Use TG_PASSWORD or the hidden interactive prompt for 2FA, and BOT_TOKEN for optional bot credentials.
