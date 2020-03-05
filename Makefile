SHELL := /bin/bash -euo pipefail
NAVIGATOR_IMAGE?=registry.k.avito.ru/avito/navigator
NAVIGATOR_IMAGE_INIT?=registry.k.avito.ru/avito/navigator-init
NAVIGATOR_IMAGE_SIDECAR?=registry.k.avito.ru/avito/navigator-sidecar
VERSION?=1.3.0

.PHONY: deps
deps:
	@go mod tidy && go mod vendor && go mod verify

.PHONY: update
update:
	@go get -mod= -u


.PHONY: test
test:
	@go test -tags unit -race -timeout 1s ${TESTFLAGS} ./...


.PHONY: build
build:
	@mkdir -p bin
	CGO_ENABLED=0 GOFLAGS="-ldflags=-w -ldflags=-s"  GO111MODULE=on go build -v -mod vendor -o ./bin/navigator ./cmd/navigator/main.go

.PHONY: build-race-detector
build-race-detector:
	@mkdir -p bin
	CGO_ENABLED=1 GOFLAGS="-ldflags=-w -ldflags=-s"  GO111MODULE=on go build -tags netgo -race -v -mod vendor -o ./bin/navigator ./cmd/navigator/main.go

.PHONY: docker-build-all
docker-build-all: docker-build-navigator docker-build-navigator-init docker-build-navigator-sidecar

.PHONY: docker-build-navigator
docker-build-navigator:
	docker build --pull -f build/control-plane/Dockerfile -t ${NAVIGATOR_IMAGE}:${VERSION} ${DOCKER_BUILD_FLAGS} .

.PHONY: docker-build-navigator-init
docker-build-navigator-init:
	docker build --pull -f build/init/Dockerfile -t ${NAVIGATOR_IMAGE_INIT}:${VERSION} ${DOCKER_BUILD_FLAGS} ./build/init

.PHONY: docker-build-navigator-sidecar
docker-build-navigator-sidecar:
	docker build --pull -f build/sidecar/Dockerfile -t ${NAVIGATOR_IMAGE_SIDECAR}:${VERSION} ${DOCKER_BUILD_FLAGS} ./build/sidecar

.PHONY: push-all
push-all:
	docker push ${NAVIGATOR_IMAGE}:${VERSION}
	docker push ${NAVIGATOR_IMAGE_INIT}:${VERSION}
	docker push ${NAVIGATOR_IMAGE_SIDECAR}:${VERSION}

.PHONY: prepare-for-e2e
prepare-for-e2e:
	./tests/e2e/setup/init-clusters.sh && ./tests/e2e/setup/build-test-rig.sh

.PHONY: test-e2e
test-e2e:
	go test -timeout 50m -count=1 -mod vendor  -v -tags e2e_test github.com/avito-tech/navigator/tests/e2e/tests ${FLAGS}

.PHONY: test-e2e-locality-disabled
test-e2e-locality-disabled:
	go test -timeout 50m -count=1 -mod vendor  -v -tags e2e_test github.com/avito-tech/navigator/tests/e2e/tests --enable-locality=false ${FLAGS}

.PHONY: test-unit
test-unit:
	@go test -cover -tags unit -timeout 30s `go list ./... | grep -v -e generated -e tests`

.PHONY: lint
lint:
	@golangci-lint run ./...

.PHONY: lint-fast
lint-fast:
	@golangci-lint run ./... --fast

.PHONY: lint-fix
lint-fix:
	@golangci-lint run ./... --fix

.PHONY: client-gen
client-gen:
	@./hack/upd_apis.sh

