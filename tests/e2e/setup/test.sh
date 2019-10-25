#!/usr/bin/env bash

cd $(dirname "$0")
source config.sh

for CLUSTER in ${CLUSTER_LIST};
do
	export KUBECONFIG="$(kind get kubeconfig-path --name="${CLUSTER}")"
	for SRC in ${NS_LIST};
	do
		for DST in ${NS_LIST};
		do
			echo -e  "\n=============== $SRC -> $DST ====================\n"
			for I in {1..4};
			do
				kubectl exec -n $SRC curl -c c curl -- -s main.$DST.svc.cluster.local:8999
			done;
		done;
	done;
done;
