#!/bin/bash

export PATH=$PATH:$(pwd)/_cache
export OO_INSTALL_NAMESPACE=openshift-metallb-operator
export USE_LOCAL_RESOURCES=true
export IS_OPENSHIFT=1

hack/validate_ocp_bundle.sh
go test --tags=validationtests -v ./test/e2e/validation -ginkgo.v -junit /logs/artifacts/ -report /logs/artifacts/
go test --tags=e2etests -v ./test/e2e/functional -ginkgo.v -ginkgo.skip "frr-k8s" -junit /logs/artifacts/ -report /logs/artifacts/
