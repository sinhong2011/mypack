# Security policy

Mypack downloads signed runtimes, installs apps and stores website sessions, so
security reports matter a lot to us.

## Reporting a vulnerability

Please **do not** open a public issue for a security problem. Use the
repository's private vulnerability reporting instead ("Security" tab → "Report a
vulnerability").

Please include:

- what an attacker could do, and what they need first (local access, a
  malicious website, a modified registry, …)
- steps to reproduce, and the Mypack version and operating system
- any proof-of-concept you have

We aim to reply within 7 days and to agree on a disclosure date with you. We
are happy to credit you in the release notes.

## Supported versions

Security fixes go into the latest release. Please update before reporting.

## Areas we care about most

- Runtime registry verification (Ed25519 signatures, manifests and downloads)
- The update, repair and rollback paths and website profile handling
- The CLI's `--json` interface, the MCP server and the browser companion's
  native messaging host
- Script, style and content-blocking injection in generated apps
