#!/bin/bash

GRAFANA_BASE_PORT=3333
PORT_MOD=1000
USER_MD5=$(echo -n ${USER} | md5sum | awk '{print $1}')
USER_PORT_MODED=$((0x${USER_MD5} % ${PORT_MOD}))
USER_GRAFANA_PORT=$((${GRAFANA_BASE_PORT} + ${USER_PORT_MODED}))

pkill -u $USER grafana
make run-frontend &
GF_SERVER_HTTP_PORT=${USER_GRAFANA_PORT} make run