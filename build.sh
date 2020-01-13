#!/bin/bash -e
cd $(dirname $0)
mkdir -p output/bin output/conf output/data output/log

set -e

GIT_SHA=`git rev-parse --short HEAD || echo "NotGitVersion"`
GIT_BRANCH=`git branch 2>/dev/null | grep "^\*" | sed -e "s/^\*\ //"`
if [ "${GIT_BRANCH}" != "" ]; then
    GIT_SHA=$GIT_SHA"($GIT_BRANCH)"
fi
WHEN=`date '+%Y-%m-%d_%H:%M:%S'`

GO111MODULE=on
go mod tidy
CGO_ENABLED=0 go build -installsuffix -a -v -o tikv_proxy -ldflags "-s -X main.GitSHA=${GIT_SHA} -X main.BuildTime=${WHEN}" .

cp run.sh output/bin/
cp clean_log.sh output/bin/
mv tikv_proxy output/bin/tikv_proxy
chmod 755 output/bin/*
cp -r conf/* output/conf/
