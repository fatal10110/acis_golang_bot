module github.com/fatal10110/acis_golang_bot

go 1.26.0

// web is the control panel's frontend source (make ui); its node_modules
// carries Go files of npm packages that are not this module's.
ignore ./web

require (
	github.com/go-sql-driver/mysql v1.10.0
	golang.org/x/crypto v0.52.0
	golang.org/x/sys v0.45.0
)

require filippo.io/edwards25519 v1.2.0 // indirect
