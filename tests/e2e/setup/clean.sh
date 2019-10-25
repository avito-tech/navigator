#!/usr/bin/env bash

cd $(dirname "$0")
source config.sh

DELETING_NS="${NS_LIST} ${NAVIGATOR_NS}"
NS_REGEX=${NAVIGATOR_NS}
for CLUSTER in ${CLUSTER_LIST};
do
	export KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"

	for NS in ${DELETING_NS}
	do
		kubectl delete ns ${NS} --wait=false || true
		NS_REGEX="$NS_REGEX|$NS"
	done
done

for CLUSTER in ${CLUSTER_LIST};
do
	export KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"
	while kubectl get ns | $GREP -P "$NS_REGEX";
	do
		sleep 1
	done;
done;