# Build Stage
FROM golang:1.24 AS build-stage

# Multi-platform build arguments
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

LABEL app="build-urlchecker"
LABEL REPO="https://github.com/extimsu/urlchecker"

ENV PROJPATH=/go/src/github.com/extimsu/urlchecker

# Because of https://github.com/docker/docker/issues/14914
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin

# Set Go environment variables for cross-compilation
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH:-amd64}
ENV CGO_ENABLED=0

ADD . ${PROJPATH}
WORKDIR ${PROJPATH}

RUN make build-alpine

# Final Stage
FROM gcr.io/distroless/base-debian11

ARG GIT_COMMIT
ARG VERSION
LABEL REPO="https://github.com/extimsu/urlchecker"
LABEL GIT_COMMIT=$GIT_COMMIT
LABEL VERSION=$VERSION

# Because of https://github.com/docker/docker/issues/14914
ENV PATH=$PATH:/opt/urlchecker/bin

WORKDIR /opt/urlchecker/bin

COPY --from=build-stage /go/src/github.com/extimsu/urlchecker/bin/urlchecker /opt/urlchecker/bin/

USER nonroot:nonroot

ENTRYPOINT ["/opt/urlchecker/bin/urlchecker"]
