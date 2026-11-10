module dendronotes

// Minimum version required by the golang.org/x modules (os.Root is complete
// since 1.25). The container image is built with the current Go release.
go 1.26.0

// The release the binaries and the image are built with (CI, SBOM).
toolchain go1.27.3

require (
	github.com/coreos/go-oidc/v3 v3.21.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/go-ldap/ldap/v3 v3.4.15
	github.com/ncruces/go-sqlite3 v0.36.0
	golang.org/x/crypto v0.58.0
	golang.org/x/oauth2 v0.37.0
	golang.org/x/term v0.47.0
	golang.org/x/text v0.43.0
	golang.org/x/time v0.16.0
)

require (
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.4.35401 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)
