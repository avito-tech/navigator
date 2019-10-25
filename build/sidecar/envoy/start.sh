#!/usr/bin/env bash

ENVOY_CONCURRENCY=${ENVOY_CONCURRENCY:-3}

sed -i -e "s/%NAVIGATOR_ADDRESS%/${NAVIGATOR_ADDRESS}/g" /envoy/envoy.yaml
sed -i -e "s/%NAVIGATOR_PORT%/${NAVIGATOR_PORT}/g" /envoy/envoy.yaml

/usr/local/bin/envoy -c /envoy/envoy.yaml --service-node ${SERVICE_NAME} --service-cluster ${CLUSTER_NAME} -l ${LOG_LEVEL} --concurrency ${ENVOY_CONCURRENCY}
