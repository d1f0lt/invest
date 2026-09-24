PORTFOLIO_SRC := ../portfolio/proto/portfolio.proto

.PHONY: proto
proto:
	mkdir -p internal/portfoliopb
	cp $(PORTFOLIO_SRC) internal/portfoliopb/portfolio.proto
	protoc -I internal/portfoliopb \
		--go_out=internal/portfoliopb --go_opt=paths=source_relative \
		--go-grpc_out=internal/portfoliopb --go-grpc_opt=paths=source_relative \
		internal/portfoliopb/portfolio.proto
	rm internal/portfoliopb/portfolio.proto

.PHONY: clean
clean:
	rm -rf internal/portfoliopb
