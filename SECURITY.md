# Security policy

## Reporting a vulnerability

Please report security problems privately through GitHub: *Security → Report a vulnerability*
in this repository (a private security advisory). Do not open a public issue.

Describe what you found, how to reproduce it and which version you use. You will get an answer
within a week; fixes are released as soon as they are ready, and the advisory is published
afterwards with credit to you, unless you prefer otherwise.

## Supported versions

Security fixes go into the latest release. Please upgrade before reporting.

## What dendronotes publishes

Every release carries a software bill of materials (CycloneDX, `sbom.cdx.json`) and
vulnerability statements (OpenVEX, `vex.openvex.json`) for findings that do not affect
dendronotes. How they are made is described in [docs/security.md](docs/security.md#supply-chain).
