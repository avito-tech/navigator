#!/usr/bin/env bash

export KUBECONFIG="$(kind get kubeconfig-path --name="$CLUSTER")"
kubectl $@
#sleep 3
#echo $KUBECONFIG $@

