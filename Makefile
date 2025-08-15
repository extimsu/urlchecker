
.PHONY: build run build-alpine clean test help default build-multiarch build-amd64 build-arm64 build-armv7



BIN_NAME=urlchecker

VERSION := $(shell grep "const Version " version/version.go | sed -E 's/.*"(.+)"$$/\1/')
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
BUILD_DATE=$(shell date '+%Y-%m-%d-%H:%M:%S')
IMAGE_NAME := "extim/urlchecker"

default: run

help:
	@echo 'Management commands for urlchecker:'
	@echo
	@echo 'Usage:'
	@echo '    make build           Compile the project.'
	@echo '    make get-deps        runs dep ensure, mostly used for ci.'
	@echo '    make build-alpine    Compile optimized for alpine linux.'
	@echo '    make package         Build final docker image with just the go binary inside'
	@echo '    make build-multiarch Build multi-architecture docker images (amd64, arm64, armv7)'
	@echo '    make build-amd64     Build amd64 docker image'
	@echo '    make build-arm64     Build arm64 docker image'
	@echo '    make build-armv7     Build armv7 docker image'
	@echo '    make tag             Tag image created by package with latest, git commit and version'
	@echo '    make test            Run tests on a compiled project.'
	@echo '    make push            Push tagged images to registry'
	@echo '    make clean           Clean the directory tree.'
	@echo

build:
	@echo "building ${BIN_NAME} ${VERSION}"
	@echo "GOPATH=${GOPATH}"
	go build -ldflags "-X github.com/extimsu/urlchecker/version.GitCommit=${GIT_COMMIT}${GIT_DIRTY} -X github.com/extimsu/urlchecker/version.BuildDate=${BUILD_DATE}" -o bin/${BIN_NAME}

run: build
	./bin/${BIN_NAME}

get-deps:
	dep ensure

build-alpine:
	@echo "building ${BIN_NAME} ${VERSION} for ${GOOS:-linux}/${GOARCH:-amd64}"
	@echo "GOPATH=${GOPATH}"
	GOOS=${GOOS:-linux} GOARCH=${GOARCH:-amd64} CGO_ENABLED=0 go build \
		-ldflags '-w -s -X github.com/extimsu/urlchecker/version.GitCommit=${GIT_COMMIT}${GIT_DIRTY} -X github.com/extimsu/urlchecker/version.BuildDate=${BUILD_DATE}' \
		-o bin/${BIN_NAME}

package:
	@echo "building image ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	docker build --build-arg VERSION=${VERSION} --build-arg GIT_COMMIT=$(GIT_COMMIT) -t $(IMAGE_NAME):local .

# Multi-architecture build targets
build-multiarch:
	@echo "building multi-architecture images ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	$(MAKE) build-amd64
	$(MAKE) build-arm64
	$(MAKE) build-armv7
	@echo "Multi-architecture images built successfully:"
	@echo "  - $(IMAGE_NAME):$(GIT_COMMIT)-amd64"
	@echo "  - $(IMAGE_NAME):$(GIT_COMMIT)-arm64"
	@echo "  - $(IMAGE_NAME):$(GIT_COMMIT)-armv7"
	@echo "  - $(IMAGE_NAME):${VERSION}-amd64"
	@echo "  - $(IMAGE_NAME):${VERSION}-arm64"
	@echo "  - $(IMAGE_NAME):${VERSION}-armv7"

build-multiarch-push:
	@echo "building and pushing multi-architecture images ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 \
		--build-arg VERSION=${VERSION} \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE_NAME):$(GIT_COMMIT) \
		-t $(IMAGE_NAME):${VERSION} \
		-t $(IMAGE_NAME):latest \
		--push .

build-amd64:
	@echo "building amd64 image ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	docker buildx build --platform linux/amd64 \
		--build-arg VERSION=${VERSION} \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE_NAME):$(GIT_COMMIT)-amd64 \
		-t $(IMAGE_NAME):${VERSION}-amd64 \
		--load .

build-arm64:
	@echo "building arm64 image ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	docker buildx build --platform linux/arm64 \
		--build-arg VERSION=${VERSION} \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE_NAME):$(GIT_COMMIT)-arm64 \
		-t $(IMAGE_NAME):${VERSION}-arm64 \
		--load .

build-armv7:
	@echo "building armv7 image ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	docker buildx build --platform linux/arm/v7 \
		--build-arg VERSION=${VERSION} \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE_NAME):$(GIT_COMMIT)-armv7 \
		-t $(IMAGE_NAME):${VERSION}-armv7 \
		--load .

tag: 
	@echo "Tagging: latest ${VERSION} $(GIT_COMMIT)"
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):$(GIT_COMMIT)
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):${VERSION}
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):latest

push: tag
	@echo "Pushing docker image to registry: latest ${VERSION} $(GIT_COMMIT)"
	docker push $(IMAGE_NAME):$(GIT_COMMIT)
	docker push $(IMAGE_NAME):${VERSION}
	docker push $(IMAGE_NAME):latest

clean:
	@test ! -e bin/${BIN_NAME} || rm bin/${BIN_NAME}

test:
	go test ./...

