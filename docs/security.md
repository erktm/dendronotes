# Security

How dendronotes protects notes and accounts. To report a vulnerability, see
[SECURITY.md](../SECURITY.md); the software bill of materials and the vulnerability statements
are described under [supply chain](#supply-chain).

## Design

- Somas are separated per account: requests for a soma the account has no access to (neither
  personally nor through a group) are answered with 404, as if it did not exist; members with
  too low a role get 403. The administration needs the admin role. A test runs every route as
  non-member and with every role, given personally and through a group.
- OpenID Connect uses the authorization code flow with PKCE (S256), a random state bound to
  the browser by a short-lived cookie (no login CSRF, each state works once) and a nonce; the
  ID token's signature, issuer, audience, expiry and nonce are checked. After sign-in the
  browser only returns to paths within dendronotes. The search for people to share with needs
  two characters and shows only user and display names (and group names).
- Every file access goes through Go's `os.Root`: paths with `..`, absolute paths and symlinks
  pointing out of the soma are refused by the kernel-level lookup. API paths are validated
  beforehand and can never address hidden entries (`.git`, `.trash`, …). Symlinks inside the
  soma are shown read-only.
- Saving requires the version the client started from (`If-Match`); changes made elsewhere in
  the meantime are never overwritten silently.
- State-changing requests from other origins are rejected (`Sec-Fetch-Site`/`Origin`); the API
  only accepts JSON bodies.
- Strict Content-Security-Policy with per-response nonce; no inline scripts, no `eval`, no
  external hosts. The only inline style allowed is the exact `tab-size: 4` attribute the
  editor sets (by hash).
- Uploads must have an allowed extension and content that matches it (a `.png` must be a
  PNG); HTML, scripts and executables are refused. Files are served with the type of their
  extension, `nosniff`, and a sandboxing Content-Security-Policy, so a file opened directly in
  the browser (an SVG, say) can never run scripts. Notes show media only from the soma.
- Drawings are SVG that the server checks on every save (`internal/drawing`): a short list of
  elements and attributes, pictures only as `data:image` URLs, no scripts, event handlers,
  `foreignObject`, external references or CSS `url()`. SVG icons pass the same check, PNG icons
  are decoded and encoded anew. In the app drawings are drawn from their stored scene, which is
  checked again (known fields, numbers in range, colors from the slots), never from the file's
  markup; files changed outside dendronotes are shown as plain images until they have a valid
  scene.
- `change` events on the event stream carry only paths and versions. Note content travels
  only to the tabs that joined that note for editing together, which needs read access to the
  soma; pushing changes needs edit access. The server checks every change set (structure,
  length against the document, size limit) before applying it, and limits how many updates
  and cursor moves a tab may send. Streams end when the session ends or the account loses
  access to the soma. Search snippets mark matches with control characters, not HTML.
- Note content is untrusted: the reading view renders it with markdown-it and sanitizes the
  result with DOMPurify (no scripts, event handlers, style attributes, forms, frames, SVG or
  `javascript:` URLs). External links open with `rel="noopener noreferrer"`. A test corpus of
  XSS payloads guards this.

## Supply chain

- **Pinned dependencies.** Go modules are vendored (`vendor/`), npm packages are locked
  (`web/package-lock.json`, installed with `npm ci`, install scripts off), base images are
  pinned by digest. Builds need no network (`make vendor`, `make image-offline`).
- **Reproducible image.** The same source yields the same image digest.
- **SBOM.** [`security/sbom.cdx.json`](../security/sbom.cdx.json) lists every component that
  ends up in the binary and image (the Go modules, the Go standard library and the npm
  packages bundled into the web interface) with version, package URL and license, in
  CycloneDX 1.6. `make sbom` writes it (`scripts/sbom.mjs`); releases attach it, and the
  container image carries an SBOM attestation as well.
- **Vulnerability statements (VEX).** [`security/vex.openvex.json`](../security/vex.openvex.json)
  says, in OpenVEX, why findings of the scanners do not affect dendronotes, for example a
  vulnerable package of a module that dendronotes does not import. `make vulncheck` runs
  govulncheck and `npm audit` and fails when a finding has no statement there
  (`scripts/vex-check.mjs`); the CI does the same on every change and every week.
- **Reporting.** See [SECURITY.md](../SECURITY.md).
