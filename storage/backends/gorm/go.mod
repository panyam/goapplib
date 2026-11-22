module github.com/panyam/goapplib/storage/backends/gorm

go 1.26.9

toolchain go1.27.4

require (
	github.com/panyam/goapplib/storage v0.7.26
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/fernet/fernet-go v0.0.0-20240119011108-303da6aec611 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/panyam/goutils v0.1.14 // indirect
	github.com/panyam/oneauth v0.3.16 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/oauth2 v0.38.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/panyam/goapplib/storage => ../..
