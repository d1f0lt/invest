.PHONY: proto
proto:
	protoc -I proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/securitiesreader.proto

.PHONY: clean
clean:
	rm -f proto/securitiesreader.pb.go proto/securitiesreader_grpc.pb.go
