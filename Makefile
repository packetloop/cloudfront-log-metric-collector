PROJECT_NAME := cloudfront-metrics-collector
package = github.com/packetloop/$(PROJECT_NAME)
FILENAME := $(PROJECT_NAME)_linux_amd64
GIT_SHA = $(shell git rev-parse --verify HEAD --short)
GIT_TAG = $(shell git describe)
GITHUB_TAG = $(shell git describe --tags)
DOCKER_REPO := arbornetworks-docker-v2.bintray.io
SCRIPT_PATH := /opt/$(PROJECT_NAME)
EXEC_ARCH := linux_amd64

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
	cd query && go test -race -cover -v . && cd ..
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
	  --build-arg GITHUB_TOKEN=$(TOKEN) .

.PHONY: push
push: bintray-login
	docker push $(DOCKER_REPO)/$(PROJECT_NAME):$(GIT_TAG)-$(GIT_SHA)

.PHONY: run
run: rundeps
	SQS_REGION=$(SQS_REGION) GoRoutine=$(GOROUTINE) \
	  SQS_QUEUE_URL=$(SQS_QUEUE_URL) \
	  STATSD_HOST=$(STATSD_HOST) \
	  CLUB_NAME=$(CLUB_NAME) \
	  $(SCRIPT_PATH)/$(PROJECT_NAME)_$(EXEC_ARCH)

.PHONY: ci-build
ci-build: dep
	@$(MAKE) compile
	ghr -t $GITHUB_TOKEN -u $CIRCLE_PROJECT_USERNAME -r $CIRCLE_PROJECT_REPONAME --replace `git describe --tags` release/
	@$(MAKE) build TOKEN=$GITHUB_TOKEN
	@$(MAKE) push

.SILENT: clean
clean:
	# Make sure our existing daemon is cleaned up after use.
	@pkill statsdaemon && echo "Clean up successful" || true

.PHONY: builddeps
builddeps:
ifndef TOKEN
	$(error TOKEN is not set)
endif

.PHONY: release
release: gittag
	git tag v$(TAG) -m "v$(TAG)"

.PHONY: gittag
gittag:
ifndef TAG
	$(error TAG is not set)
endif

.PHONY: rundeps
rundeps:
	ifndef GOROUTINE
	       $(error GOROUTINE is not set and must be >= 3)
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
