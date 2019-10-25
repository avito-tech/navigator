FROM golang:1.13

MAINTAINER Mikhail Shaverdo <m.shaverdo@gmail.com>

COPY ./ /navigator

WORKDIR /navigator

RUN mkdir -p /navigator/bin/ ; \
	CGO_ENABLED=0 GOOS=linux GOFLAGS="-ldflags=-w -ldflags=-s"  go build  -mod vendor -o ./bin/probe /navigator/tests/e2e/rig/probe.go ;\
	CGO_ENABLED=0 GOOS=linux GOFLAGS="-ldflags=-w -ldflags=-s"  go build  -mod vendor -o ./bin/probe-server /navigator/tests/e2e/rig/probe-server.go

FROM alpine

RUN apk add curl

COPY --from=0 /navigator/bin/* /