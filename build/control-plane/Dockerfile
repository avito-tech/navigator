FROM golang:1.14

ARG NAVIGATOR_BUILD_TARGET=build

WORKDIR /navigator

COPY ./ /navigator

RUN make $NAVIGATOR_BUILD_TARGET

FROM debian:stretch

COPY --from=0 /navigator/bin/navigator /bin/navigator

ENTRYPOINT ["navigator"]
