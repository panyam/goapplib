module github.com/panyam/goapplib/storage/drivers/msgraph

go 1.26.9

toolchain go1.27.4

require (
	github.com/panyam/goapplib/storage v0.7.26
	golang.org/x/oauth2 v0.38.0
)

require (
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/fernet/fernet-go v0.0.0-20240119011108-303da6aec611 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/panyam/goutils v0.1.14 // indirect
	github.com/panyam/oneauth v0.3.16 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/panyam/goapplib/storage => ../..
