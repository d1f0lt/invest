.PHONY: proto
proto:
	protoc -I proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/feed.proto

.PHONY: clean
clean:
	rm -f proto/feed.pb.go proto/feed_grpc.pb.go
