# syntax=docker/dockerfile:1
# Local and CI image build. Releases use Dockerfile.release with the binaries
# GoReleaser already built.
FROM --platform=$BUILDPLATFORM golang:1.27@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags "-s -w \
      -X github.com/extimsu/urlchecker/version.Version=${VERSION} \
      -X github.com/extimsu/urlchecker/version.GitCommit=${COMMIT} \
      -X github.com/extimsu/urlchecker/version.BuildDate=${DATE}" \
    -o /out/urlchecker .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/urlchecker /usr/local/bin/urlchecker
USER 65532:65532
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/urlchecker"]
