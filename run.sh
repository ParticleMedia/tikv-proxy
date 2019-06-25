#!/bin/bash -e
cd $(dirname $0)

LOG_DIR=../log
CONF_PATH=../conf/tikv_proxy.yaml
LOG_LEVEL=3
ALSO_LOG_TO_STDERR=false
PUSH_TSDB=true

mkdir -p ${LOG_DIR}
echo $$ >./pid
exec ./tikv_proxy -conf=${CONF_PATH} \
    -log_dir=${LOG_DIR} \
    -v=${LOG_LEVEL} \
    -alsologtostderr=${ALSO_LOG_TO_STDERR} \
    -metrics_to_tsdb=${PUSH_TSDB}
rm -f ./pid
