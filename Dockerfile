# Build the manager binary
FROM golang:1.13 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/
COPY bindata/deployment/ bindata/deployment/
COPY bindata/configuration/address-pool/ bindata/configuration/address-pool/
COPY bindata/configuration/bgp-peer/ bindata/configuration/bgp-peer/
COPY .git/ .git/
COPY Makefile Makefile

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -ldflags "-X main.build=$(git rev-parse HEAD)" -o manager main.go 

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/bindata/deployment /bindata/deployment
COPY --from=builder /workspace/bindata/configuration/address-pool/ /bindata/configuration/address-pool
COPY --from=builder /workspace/bindata/configuration/bgp-peer/ /bindata/configuration/bgp-peer

USER nonroot:nonroot

ENTRYPOINT ["/manager"]
