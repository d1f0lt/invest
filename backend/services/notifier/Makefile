.PHONY: proto
proto:
	protoc -I proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/notifier.proto

.PHONY: clean
clean:
	rm -f proto/notifier.pb.go proto/notifier_grpc.pb.go
