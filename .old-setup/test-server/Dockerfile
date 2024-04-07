ARG GO_VERSION
FROM golang:1.21.1 as build-env

WORKDIR /go/src/app
COPY go.* ./

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
RUN go env
COPY ./ /go/src/app
# RUN CGO_ENABLED=0 GO111MODULE=on go build -o /go/bin/app ./cmd/test-server/
RUN CGO_ENABLED=0 GO111MODULE=on go build -o /go/bin/app .

# -------------------------------------------------------
FROM gcr.io/distroless/base as run-env

COPY --from=build-env /go/bin/app /
USER 65534:65534
ENTRYPOINT ["/app"]