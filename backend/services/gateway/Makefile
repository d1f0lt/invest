USERS_SRC := ../users/proto/users.proto
PORTFOLIO_SRC := ../portfolio/proto/portfolio.proto
SECURITIESREADER_SRC := ../securities_reader/proto/securitiesreader.proto

.PHONY: proto
proto: proto-users proto-portfolio proto-securitiesreader

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

.PHONY: proto-securitiesreader
proto-securitiesreader:
	mkdir -p internal/securitiesreaderpb
	cp $(SECURITIESREADER_SRC) internal/securitiesreaderpb/securitiesreader.proto
	protoc -I internal/securitiesreaderpb \
		--go_out=internal/securitiesreaderpb --go_opt=paths=source_relative \
		--go-grpc_out=internal/securitiesreaderpb --go-grpc_opt=paths=source_relative \
		internal/securitiesreaderpb/securitiesreader.proto
	rm internal/securitiesreaderpb/securitiesreader.proto

.PHONY: clean
clean: clean-users clean-portfolio clean-securitiesreader

.PHONY: clean-users
clean-users:
	rm -rf internal/userspb

.PHONY: clean-portfolio
clean-portfolio:
	rm -rf internal/portfoliopb

.PHONY: clean-securitiesreader
clean-securitiesreader:
	rm -rf internal/securitiesreaderpb
