#!/usr/bin/env bash

IFS=,

# these ports work for netra
INBOUND_INTERCEPT_PORTS=${INBOUND_INTERCEPT_PORTS:-" "}
OUTBOUND_INTERCEPT_PORTS=${OUTBOUND_INTERCEPT_PORTS:-" "}

NETRA_SIDECAR_PORT=${NETRA_SIDECAR_PORT:-14956}
NETRA_SIDECAR_USER_ID=${NETRA_SIDECAR_USER_ID:-1337}
ENVOY_SIDECAR_PORT=${ENVOY_SIDECAR_PORT:-15001}
ENVOY_SIDECAR_USER_ID=${ENVOY_SIDECAR_USER_ID:-1338}

CLUSTER_SUBNETS=${CLUSTER_SUBNETS:-"10.100.0.0/16"}

function dump {
    iptables-save
}

trap dump EXIT

iptables -t nat -A OUTPUT -m owner --uid-owner ${ENVOY_SIDECAR_USER_ID} -j RETURN
for SUBNET in ${CLUSTER_SUBNETS};
do
  iptables -t nat -A OUTPUT -d ${SUBNET} -p tcp -m owner --uid-owner ${NETRA_SIDECAR_USER_ID} -j REDIRECT --to-ports ${ENVOY_SIDECAR_PORT}
done;

iptables -t nat -A OUTPUT -p tcp -m owner --uid-owner ${NETRA_SIDECAR_USER_ID} -j RETURN
iptables -t nat -A OUTPUT -d 127.0.0.1/32 -o lo -p tcp -j RETURN
if [ "${OUTBOUND_INTERCEPT_PORTS}" == "*" ]; then
    iptables -t nat -A OUTPUT -p tcp -j REDIRECT --to-ports ${NETRA_SIDECAR_PORT}
else
    for port in ${OUTBOUND_INTERCEPT_PORTS}; do
        iptables -t nat -A OUTPUT -p tcp --dport ${port} -j REDIRECT --to-ports ${NETRA_SIDECAR_PORT} || true
    done
fi

for SUBNET in ${CLUSTER_SUBNETS};
do
  iptables -t nat -A OUTPUT -d ${SUBNET} -p tcp -j REDIRECT --to-ports ${ENVOY_SIDECAR_PORT}
done;

# inbound for netra
iptables -t nat -A PREROUTING
if [ "${INBOUND_INTERCEPT_PORTS}" == "*" ]; then
    iptables -t nat -A PREROUTING -p tcp -m tcp -j REDIRECT --to-ports ${NETRA_SIDECAR_PORT}
else
    for port in ${INBOUND_INTERCEPT_PORTS}; do
        iptables -t nat -A PREROUTING -p tcp -m tcp --dport ${port} -j REDIRECT --to-ports ${NETRA_SIDECAR_PORT} || true
    done
fi
