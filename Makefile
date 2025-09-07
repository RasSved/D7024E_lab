all: build
.PHONY: all build container push coverage test install

BINARY_NAME ?= kadlab           
BUILD_IMAGE ?= kadlab:latest    
PUSH_IMAGE  ?= RasSved/kadlab:v1.0.0

VERSION   := $(shell git rev-parse --short HEAD)
BUILDTIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ)

GOLDFLAGS += -X 'main.BuildVersion=$(VERSION)'
GOLDFLAGS += -X 'main.BuildTime=$(BUILDTIME)'

build:
	@echo "==> building linux/amd64 binary to ./bin/$(BINARY_NAME)"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w $(GOLDFLAGS)" -o ./bin/$(BINARY_NAME) ./cmd/main.go

container: build
	@echo "==> building image $(BUILD_IMAGE)"
	docker build -t $(BUILD_IMAGE) .

push:
	docker tag $(BUILD_IMAGE) $(PUSH_IMAGE)
	docker push $(BUILD_IMAGE)
	docker push $(PUSH_IMAGE)

coverage:
	./buildtools/coverage.sh
	./buildtools/codecov

test:
	@go test -v -race ./pkg/helloworld

install:
	cp ./bin/$(BINARY_NAME) /usr/local/bin
