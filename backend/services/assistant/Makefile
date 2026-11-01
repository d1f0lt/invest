PORTFOLIO_SRC := ../portfolio/proto/portfolio.proto
FEED_SRC := ../feed/proto/feed.proto

.PHONY: proto clean
proto: proto-portfolio proto-feed

clean: clean-portfolio clean-feed

.PHONY: proto-portfolio
proto-portfolio:
	mkdir -p internal/portfoliopb
	cp $(PORTFOLIO_SRC) internal/portfoliopb/portfolio.proto
	protoc -I internal/portfoliopb \
		--go_out=internal/portfoliopb --go_opt=paths=source_relative \
		--go-grpc_out=internal/portfoliopb --go-grpc_opt=paths=source_relative \
		internal/portfoliopb/portfolio.proto
	rm internal/portfoliopb/portfolio.proto

.PHONY: clean-portfolio
clean-portfolio:
	rm -rf internal/portfoliopb

.PHONY: proto-feed
proto-feed:
	mkdir -p internal/feedpb
	cp $(FEED_SRC) internal/feedpb/feed.proto
	protoc -I internal/feedpb \
		--go_out=internal/feedpb --go_opt=paths=source_relative \
		--go-grpc_out=internal/feedpb --go-grpc_opt=paths=source_relative \
		internal/feedpb/feed.proto
	rm internal/feedpb/feed.proto

.PHONY: clean-feed
clean-feed:
	rm -rf internal/feedpb
