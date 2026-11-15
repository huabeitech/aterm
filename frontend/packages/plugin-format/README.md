# Plugin package format

Internal shared source for the host and CLI: manifest/capability validation, bounded directory inspection, and ZIP transport. The CLI bundles these modules into its published distribution; it does not import host internals or require this repository at runtime.

A `.aterm-plugin` is a ZIP containing only `manifest.json`, `plugin.js`, and manifest-declared `ui/*.html` and `locales/*.json`. No enclosing directory. Archives are read into bounded memory, reject duplicate/undeclared paths, encrypted entries, links and nonregular files, and verify CRC before using the existing directory validator. Installed content continues to use the host's SHA-256 integrity checks. Neither CRC nor the content hash establishes publisher identity.

Limits: archive and expanded total 16 MiB, 128 entries, 2 MiB per entry; directory validation additionally limits manifest to 32 KiB, Worker to 256 KiB, each locale to 64 KiB / total 512 KiB, each HTML to 2 MiB / total 8 MiB. Protocol changes belong here so build-time and install-time rules stay aligned.
