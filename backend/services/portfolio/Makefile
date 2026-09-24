.PHONY: proto
proto:
	protoc -I proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/portfolio.proto

.PHONY: clean
clean:
	rm -f proto/portfolio.pb.go proto/portfolio_grpc.pb.go
