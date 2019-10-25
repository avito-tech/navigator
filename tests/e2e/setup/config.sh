#!/usr/bin/env bash

set -eo pipefail

##################### configurable vars #######################

export CLUSTER_PREFIX="cluster-"
[ -z "$CLUSTER_COUNT" ]  && export CLUSTER_COUNT=2

export NS_PREFIX="test-"
[ -z "$NS_COUNT" ]  && export NS_COUNT=3

export POD_SUBNET_TPL='10.244.$N.0/24' # single-quoted! will be evaluated later
export NAVIGATOR_NS="navigator-system"

export VERSION="1"
export TEST_RIG_IMAGE="navigator-test-rig"
export NAVIGATOR_IMAGE_INIT="init"
export NAVIGATOR_IMAGE_SIDECAR="sidecar"
export NAVIGATOR_IMAGE="navigator"


##################### derived helper vars #######################
export NAVIGATOR_ROOT="../../../"
export CONFIG_DIR="${NAVIGATOR_ROOT}/bin/configs"

if [ -z "$CLUSTER_LIST" ];
then
	export CLUSTER_LIST=""
	for I in $(seq 1 ${CLUSTER_COUNT}); do CLUSTER_LIST="${CLUSTER_LIST} ${CLUSTER_PREFIX}${I}"; done
fi

if [ -z "$NS_LIST" ];
then
	export NS_LIST=""
	for I in $(seq 1 ${NS_COUNT}); do NS_LIST="${NS_LIST} ${NS_PREFIX}${I}"; done
fi

case $(uname -s) in
	*[Dd]arwin* | *BSD* )
		export SED=gsed
		export GREP=ggrep
		;;
	*)
		export SED=sed
		export GREP=grep
		;;
esac
