#!/usr/bin/env bash

cd $(dirname "$0")
source config.sh

mkdir -p "$CONFIG_DIR"

for CLUSTER in ${CLUSTER_LIST};
do
	CONTAINER=`docker ps | grep ${CLUSTER} | awk '{ print $1}'`
	IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${CONTAINER})

	cp "$(kind get kubeconfig-path --name="${CLUSTER}")" "${CONFIG_DIR}/${CLUSTER}"
	$SED -i -r "s#server: https://127.0.0.1:[[:digit:]]+#server: https://${IP}:6443#g" "${CONFIG_DIR}/${CLUSTER}"
done;

cd ${NAVIGATOR_ROOT}
export DOCKER_BUILD_FLAGS='--build-arg NAVIGATOR_BUILD_TARGET=build-race-detector'
make docker-build-all
docker build -t ${TEST_RIG_IMAGE}:${VERSION} -f tests/e2e/rig/Dockerfile ./

for CLUSTER in ${CLUSTER_LIST};
do
  for IMAGE in ${NAVIGATOR_IMAGE} ${NAVIGATOR_IMAGE_INIT} ${NAVIGATOR_IMAGE_SIDECAR} ${TEST_RIG_IMAGE};
  do
	  kind load docker-image --name ${CLUSTER} ${IMAGE}:${VERSION}
	done
done
