#!/usr/bin/env bash

cd $(dirname "$0")
source config.sh

for N in $(seq 1 ${CLUSTER_COUNT});
do
	CLUSTER="${CLUSTER_PREFIX}${N}"
	POD_SUBNET=$(eval echo ${POD_SUBNET_TPL})

	$SED "s#%POD_SUBNET%#${POD_SUBNET}#g" kind-config.yaml.tpl > /tmp/kind-config.yaml
	kind delete cluster --name=${CLUSTER} || true
	kind create cluster --name=${CLUSTER} --config /tmp/kind-config.yaml

	export KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"
	echo ../../../crds/*.crd.yaml | xargs -n 1 kubectl apply -f
done

for CLUSTER in ${CLUSTER_LIST};
do
	CONTAINER=`docker ps | grep ${CLUSTER} | awk '{ print $1}'`
	for N in $(seq 1 ${CLUSTER_COUNT});
	do
		PEER_CLUSTER="${CLUSTER_PREFIX}${N}"
		PEER_CONTAINER=`docker ps | grep ${PEER_CLUSTER} | awk '{ print $1}'`
		PEER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${PEER_CONTAINER})
		PEER_POD_SUBNET=$(eval echo ${POD_SUBNET_TPL})
		docker exec ${CONTAINER} ip route add ${PEER_POD_SUBNET} via ${PEER_IP}
	done;
done
