module github.com/panyam/goapplib/docsite

go 1.26.3

require github.com/panyam/s3gen v0.1.6

require (
	connectrpc.com/connect v1.19.1 // indirect
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/adrg/frontmatter v0.2.0 // indirect
	github.com/alecthomas/chroma v0.10.0 // indirect
	github.com/alexedwards/scs/v2 v2.8.0 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fernet/fernet-go v0.0.0-20240119011108-303da6aec611 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/morrisxyang/xreflect v0.0.0-20231001053442-6df0df9858ba // indirect
	github.com/panyam/goutils v0.1.14 // indirect
	github.com/panyam/oneauth v0.1.13 // indirect
	github.com/panyam/templar v0.1.0 // indirect
	github.com/radovskyb/watcher v1.0.7 // indirect
	github.com/yuin/goldmark v1.8.2 // indirect
	github.com/yuin/goldmark-highlighting v0.0.0-20220208100518-594be1970594 // indirect
	go.abhg.dev/goldmark/anchor v0.2.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.2 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/panyam/goapplib v0.6.8
	google.golang.org/protobuf v1.36.11
)

// The demos build against the goapplib in this checkout, so the site documents main.
replace github.com/panyam/goapplib => ../
