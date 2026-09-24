.PHONY: proto
proto:
	protoc -I proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/users.proto

.PHONY: clean
clean:
	rm -f proto/users.pb.go proto/users_grpc.pb.go
