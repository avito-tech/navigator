#!/usr/bin/env bash

set -euxo pipefail

CURDIR=$(dirname $(realpath $BASH_SOURCE))
VERSION=$(go list -m all | grep k8s.io/code-generator | rev | cut -d"-" -f1 | cut -d" " -f1 | rev)
TMP_DIR=$(mktemp -d)
GOPATH=$(mktemp -d)
mkdir -p $GOPATH/{src,bin,pkg}
echo $GOPATH
GO111MODULE=off go get k8s.io/code-generator/cmd/client-gen
git clone https://github.com/kubernetes/code-generator.git ${TMP_DIR}
(cd ${TMP_DIR} && git reset --hard ${VERSION})
${TMP_DIR}/generate-groups.sh \
  all \
  github.com/avito-tech/navigator/pkg/apis/generated \
  github.com/avito-tech/navigator/pkg/apis \
  navigator:v1 \
  $@

GENERATED_DIR="$(dirname $0)/../pkg/apis/generated"
APIS_DIR="$(dirname $0)/../pkg/apis"
mv "$GENERATED_DIR" "$GENERATED_DIR.bak" || true
mv "$GOPATH/src/github.com/avito-tech/navigator/pkg/apis/generated" "$GENERATED_DIR"
cp -r "$GOPATH/src/github.com/avito-tech/navigator/pkg/apis/" "$APIS_DIR"
