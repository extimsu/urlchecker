# Build Stage
FROM golang:1.22 AS build-stage

LABEL app="build-urlchecker"
LABEL REPO="https://github.com/extimsu/urlchecker"

ENV PROJPATH=/go/src/github.com/extimsu/urlchecker

# Because of https://github.com/docker/docker/issues/14914
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin

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
