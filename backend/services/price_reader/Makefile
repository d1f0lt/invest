.PHONY: proto
proto:
	protoc -I proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/pricereader.proto

.PHONY: clean
clean:
	rm -f proto/pricereader.pb.go proto/pricereader_grpc.pb.go
