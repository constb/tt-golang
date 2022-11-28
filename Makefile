PORT=3000

proto:
	protoc --proto_path=internal/proto --go_out=paths=source_relative:internal/proto api.proto

build:
	go build -o webservice ./cmd/webservice.go

run: build
	./webservice

test:
	go test ./...

coverage:
	go test ./... -cover -coverprofile=coverage.out
	go tool cover -html=coverage.out

dep:
	go mod download

vet:
	go vet ./...

lint:
	golangci-lint run --enable-all \
		-D maligned,exhaustivestruct,scopelint,interfacer,deadcode,golint,ifshort,nosnakecase,structcheck,varcheck \
		-D rowserrcheck,sqlclosecheck,structcheck,wastedassign \
		-D funlen,gochecknoinits,lll,nestif,wsl,exhaustruct,forcetypeassert,gochecknoglobals,nlreturn,paralleltest,testpackage,varnamelen,wrapcheck \
		./...

format:
	find . -type f -name '*.go' -exec gofumpt -w {} +

clean:
	go clean
