USERS_SRC := ../users/proto/users.proto
PORTFOLIO_SRC := ../portfolio/proto/portfolio.proto
PRICEREADER_SRC := ../price_reader/proto/pricereader.proto

.PHONY: proto
proto: proto-users proto-portfolio proto-pricereader

.PHONY: proto-users
proto-users:
	mkdir -p internal/userspb
	cp $(USERS_SRC) internal/userspb/users.proto
	protoc -I internal/userspb \
		--go_out=internal/userspb --go_opt=paths=source_relative \
		--go-grpc_out=internal/userspb --go-grpc_opt=paths=source_relative \
		internal/userspb/users.proto
	rm internal/userspb/users.proto

.PHONY: proto-portfolio
proto-portfolio:
	mkdir -p internal/portfoliopb
	cp $(PORTFOLIO_SRC) internal/portfoliopb/portfolio.proto
	protoc -I internal/portfoliopb \
		--go_out=internal/portfoliopb --go_opt=paths=source_relative \
		--go-grpc_out=internal/portfoliopb --go-grpc_opt=paths=source_relative \
		internal/portfoliopb/portfolio.proto
	rm internal/portfoliopb/portfolio.proto

.PHONY: proto-pricereader
proto-pricereader:
	mkdir -p internal/pricereaderpb
	cp $(PRICEREADER_SRC) internal/pricereaderpb/pricereader.proto
	protoc -I internal/pricereaderpb \
		--go_out=internal/pricereaderpb --go_opt=paths=source_relative \
		--go-grpc_out=internal/pricereaderpb --go-grpc_opt=paths=source_relative \
		internal/pricereaderpb/pricereader.proto
	rm internal/pricereaderpb/pricereader.proto

.PHONY: clean
clean: clean-users clean-portfolio clean-pricereader

.PHONY: clean-users
clean-users:
	rm -rf internal/userspb

.PHONY: clean-portfolio
clean-portfolio:
	rm -rf internal/portfoliopb

.PHONY: clean-pricereader
clean-pricereader:
	rm -rf internal/pricereaderpb
