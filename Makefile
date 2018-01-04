PROJECT_NAME := cloudfront-metrics-collector
package = github.com/packetloop/$(PROJECT_NAME)
FILENAME := $(PROJECT_NAME)_linux_amd64
GIT_SHA = $(shell git rev-parse --verify HEAD --short)
GIT_TAG = $(shell git describe)
GITHUB_TAG = $(shell git describe --tags)
DOCKER_REPO := arbornetworks-docker-v2.bintray.io
SCRIPT_PATH := /opt/$(PROJECT_NAME)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

all:  test compile docker-build

.PHONY: dep
dep:
	go get github.com/tcnksm/ghr
	go get github.com/mitchellh/gox
	go get github.com/bitly/statsdaemon
	go get -v ./...
	go fmt ./...

.PHONY: compile
compile: dep
	gox -output="./release/{{.Dir}}_{{.OS}}_{{.Arch}}" -os="linux windows darwin" -arch="amd64" .

.PHONY: test
test: clean dep
	go test -race -cover -v ./...
	statsdaemon --address 127.0.0.1:8125 -graphite - & go test -v -race -cover .

.PHONY: bintray-login
bintray-login:
	@docker login -e ${BINTRAY_EMAIL} -u ${BINTRAY_USER} -p ${BINTRAY_API_KEY} ${DOCKER_REPO}

.PHONY: docker-build
docker-build: builddeps bintray-login
	# Suppress output with @ to hide token value from stdout.
	@docker build -t $(DOCKER_REPO)/$(PROJECT_NAME):$(GIT_TAG)-$(GIT_SHA) \
	  --build-arg GITHUB_TAG=$(GITHUB_TAG) \
	  --build-arg GITHUB_ASSET_FILENAME=$(FILENAME) \
	  --build-arg GITHUB_TOKEN=$(GITHUB_TOKEN) .

.PHONY: push
push: bintray-login
	docker push $(DOCKER_REPO)/$(PROJECT_NAME):$(GIT_TAG)-$(GIT_SHA)

.PHONY: start
start: rundeps
	SQS_REGION=$(SQS_REGION) GOROUTINE=$(GOROUTINE) \
	  SQS_QUEUE_URL=$(SQS_QUEUE_URL) \
	  STATSD_HOST=$(STATSD_HOST) \
	  CLUB_NAME=$(CLUB_NAME) \
	  $(SCRIPT_PATH)/$(PROJECT_NAME)_$(GOOS)_$(GOARCH)

.PHONY: run
run: rundeps
	SQS_REGION=$(SQS_REGION) GOROUTINE=$(GOROUTINE) \
	  SQS_QUEUE_URL=$(SQS_QUEUE_URL) \
	  STATSD_HOST=$(STATSD_HOST) \
	  CLUB_NAME=$(CLUB_NAME) \
	  go run

.PHONY: ci-build
ci-build: dep
	@$(MAKE) compile
	@ghr -t $(GITHUB_TOKEN) -u $(CIRCLE_PROJECT_USERNAME) -r $(CIRCLE_PROJECT_REPONAME) --replace `git describe --tags` release/
	@$(MAKE) docker-build GITHUB_TOKEN=$(GITHUB_TOKEN)
	@$(MAKE) push

.SILENT: clean
clean:
	# Make sure our existing daemon is cleaned up after use.
	@pkill statsdaemon && echo "Clean up successful" || true

.PHONY: builddeps
builddeps:
ifndef GITHUB_TOKEN
	$(error GITHUB_TOKEN is not set)
endif
ifndef CIRCLE_PROJECT_USERNAME
	$(error CIRCLE_PROJECT_USERNAME is not set)
endif
ifndef CIRCLE_PROJECT_REPONAME
	$(error CIRCLE_PROJECT_REPONAME is not set)
endif

.PHONY: release
release: gittag
	git tag v$(TAG) -m "v$(TAG)"
	git push --tags packetloop

.PHONY: gittag
gittag:
ifndef TAG
	$(error TAG is not set)
endif

.PHONY: rundeps
rundeps:
ifndef GOROUTINE
	$(error GOROUTINE is not set)
endif
ifndef CLUB_NAME
	$(error CLUB_NAME is not set)
endif
ifndef SQS_QUEUE_URL
	$(error SQS_QUEUE_URL is not set)
endif
ifndef SQS_REGION
	$(error SQS_REGION is not set)
endif
ifndef STATSD_HOST
	$(error STATSD_HOST is not set)
endif
