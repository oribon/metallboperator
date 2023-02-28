#!/bin/bash

set -e

pushd .
cd "$(dirname "$0")/.."

function finish {
    popd
}
trap finish EXIT

GOPATH="${GOPATH:-~/go}"
export GOFLAGS="${GOFLAGS:-"-mod=vendor"}"

export PATH=$PATH:$GOPATH/bin

mkdir -p _cache

export METALLB_COMMIT_ID="a917ee063c03ab4f465d142f5fb242d390c842b1"
export METALLB_PATH=_cache/metallb

export METALLB_SC_FILE=$(dirname "$0")/securityContext.yaml

function fetch_metallb() {
    if [[ ! -d "$METALLB_PATH" ]]; then
        curl -L https://github.com/openshift/metallb/tarball/"$METALLB_COMMIT_ID" | tar zx -C _cache
        rm -rf "$METALLB_PATH"
        mv _cache/openshift-metallb-* "$METALLB_PATH"
    fi
}
