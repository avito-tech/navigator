#!/usr/bin/env bash

set -exo pipefail

cd "$(dirname "$0")"
pwd
source config.sh

if [ "$ENABLE_LOCALITY" != "true" ];
then
  ENABLE_LOCALITY="false"
fi

export NAVIGATOR_CONFIGS=""
for CLUSTER in ${CLUSTER_LIST};
do
	NAVIGATOR_CONFIGS="${NAVIGATOR_CONFIGS} '${CLUSTER}',"
done;

for CLUSTER in ${CLUSTER_LIST};
do
	export KUBECONFIG NAVIGATOR_ADDRESS
	KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"
	NAVIGATOR_ADDRESS=$(kubectl get nodes -o jsonpath='{$.items[*].status.addresses[?(@.type=="InternalIP")].address}')

	kubectl cluster-info

	kubectl create ns ${NAVIGATOR_NS}

  kubectl create configmap -n ${NAVIGATOR_NS} cluster-configs --from-file=${CONFIG_DIR}

	$SED  "s#%NAVIGATOR_CONFIGS%#$NAVIGATOR_CONFIGS#g" navigator.yaml  > /tmp/navigator.yaml
	$SED -i "s#%VERSION%#$VERSION#g"  /tmp/navigator.yaml
	$SED -i "s#%NAVIGATOR_IMAGE_INIT%#$NAVIGATOR_IMAGE_INIT#g"  /tmp/navigator.yaml
	$SED -i "s#%NAVIGATOR_IMAGE_SIDECAR%#$NAVIGATOR_IMAGE_SIDECAR#g"  /tmp/navigator.yaml
	$SED -i "s#%NAVIGATOR_IMAGE%#$NAVIGATOR_IMAGE#g"  /tmp/navigator.yaml
	$SED -i "s#%TEST_RIG_IMAGE%#$TEST_RIG_IMAGE#g"  /tmp/navigator.yaml
	$SED -i "s#%NAVIGATOR_NS%#$NAVIGATOR_NS#g"  /tmp/navigator.yaml
	$SED -i "s#%ENABLE_LOCALITY%#$ENABLE_LOCALITY#g"  /tmp/navigator.yaml
	kubectl apply -n ${NAVIGATOR_NS} -f /tmp/navigator.yaml

	for NS in ${NS_LIST};
	do
		cp services.yaml /tmp/
		$SED -i "s#%CLUSTER_NAME%#$CLUSTER#g" /tmp/services.yaml
		$SED -i "s/%NAMESPACE%/$NS/g" /tmp/services.yaml
		$SED -i "s/%NAVIGATOR_ADDRESS%/$NAVIGATOR_ADDRESS/g" /tmp/services.yaml
		$SED -i "s/%NS_LIST%/$NS_LIST/g" /tmp/services.yaml
    $SED -i "s#%VERSION%#$VERSION#g"  /tmp/services.yaml
    $SED -i "s#%NAVIGATOR_IMAGE_INIT%#$NAVIGATOR_IMAGE_INIT#g"  /tmp/services.yaml
    $SED -i "s#%NAVIGATOR_IMAGE_SIDECAR%#$NAVIGATOR_IMAGE_SIDECAR#g"  /tmp/services.yaml
    $SED -i "s#%NAVIGATOR_IMAGE%#$NAVIGATOR_IMAGE#g"  /tmp/services.yaml
		$SED -i "s/%TEST_RIG_IMAGE%/$TEST_RIG_IMAGE/g" /tmp/services.yaml

		kubectl create ns $NS
		kubectl -n $NS apply -f /tmp/services.yaml
	done;
done;

set +o pipefail

echo "waiting pods start..."

for CLUSTER in ${CLUSTER_LIST};
do
	KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"
	while kubectl get po  --all-namespaces | grep -v STATUS |grep -v Running;
	do
		ERRNS="$(kubectl get po  --all-namespaces | grep Error | tail -n 1 | awk '{ print $1 }')"
		ERR="$(kubectl get po  --all-namespaces  | grep Error | tail -n 1 | awk '{ print $2 }')"
		if [[ "$ERR" ]];
		then
			echo kubectl -n "$ERRNS" logs -f "$ERR"
			kubectl -n "$ERRNS" logs -f "$ERR"
		fi
		sleep 5;
	done
done

echo -e "\n\n=============\n\n"
for CLUSTER in ${CLUSTER_LIST};
do
	KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"
	kubectl get svc -o wide --all-namespaces
	echo -e "\n-----------\n"
done;