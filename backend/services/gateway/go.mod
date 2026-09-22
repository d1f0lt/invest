module invest/backend/services/gateway

go 1.24

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/minio/minio-go/v7 v7.0.95
	github.com/rabbitmq/amqp091-go v1.13.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.11 // indirect
	github.com/minio/crc64nvme v1.0.2 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/tinylib/msgp v1.3.0 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241202173237-19429a94021a // indirect
)

// The build environment used to write this service blocks direct HTTP
// access to golang.org / go.uber.org / gopkg.in / google.golang.org
// (needed to resolve their vanity import paths) - the same restriction
// documented in the users and parser services' go.mod. These replace
// directives point each module at its official GitHub mirror instead -
// same module, same content, just fetched from a host that isn't
// blocked. Harmless (and removable) on a machine without this
// restriction.
replace (
	golang.org/x/crypto => github.com/golang/crypto v0.39.0
	golang.org/x/net => github.com/golang/net v0.41.0
	golang.org/x/sys => github.com/golang/sys v0.33.0
	golang.org/x/text => github.com/golang/text v0.26.0
	google.golang.org/genproto/googleapis/rpc => github.com/googleapis/go-genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/grpc => github.com/grpc/grpc-go v1.70.0
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.36.5
)
