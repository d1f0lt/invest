module invest/backend/services/users

go 1.24

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/lib/pq v1.10.9
	golang.org/x/crypto v0.39.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
)

require (
	golang.org/x/net v0.32.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241202173237-19429a94021a // indirect
)

// Same golang.org/google.golang.org vanity-import-blocking workaround as
// every other go.mod in this project (see backend/services/gateway/go.mod's comment
// for the full explanation) - extended here to also cover
// golang.org/x/crypto, needed for password hashing.
replace (
	golang.org/x/crypto => github.com/golang/crypto v0.31.0
	golang.org/x/net => github.com/golang/net v0.41.0
	golang.org/x/sys => github.com/golang/sys v0.33.0
	golang.org/x/text => github.com/golang/text v0.26.0
	google.golang.org/genproto/googleapis/rpc => github.com/googleapis/go-genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/grpc => github.com/grpc/grpc-go v1.70.0
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.36.5
)
