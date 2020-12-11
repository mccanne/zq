export GO111MODULE=on

# If VERSION or LDFLAGS change, please also change
# npm/build.
VERSION = $(shell git describe --tags --dirty --always)
ECR_VERSION = $(VERSION)-$(ZQD_K8S_USER)
LDFLAGS = -s -X github.com/brimsec/zq/cli.Version=$(VERSION)
ZEEKTAG := $(shell python -c 'import json ;print(json.load(open("package.json"))["brimDependencies"]["zeekTag"])')
ZEEKPATH = zeek-$(ZEEKTAG)
SURICATATAG := $(shell python -c 'import json; print(json.load(open("package.json"))["brimDependencies"]["suricataTag"])')
SURICATAPATH = suricata-$(SURICATATAG)

# This enables a shortcut to run a single test from the ./ztests suite, e.g.:
#  make TEST=TestZq/ztests/suite/cut/cut
ifneq "$(TEST)" ""
test-one: test-run
endif

# Uncomment this to trigger re-builds of the peg files when the grammar
# is out of date.  We are commenting this out to work around issue #1717.
#PEG_DEP=peg

vet:
	@go vet -composites=false -stdmethods=false ./...

fmt:
	@res=$$(go fmt ./...); \
	if [ -n "$${res}" ]; then \
		echo "go fmt failed on these files:"; echo "$${res}"; echo; \
		exit 1; \
	fi

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

SAMPLEDATA:=zq-sample-data/README.md

$(SAMPLEDATA):
	git clone --depth=1 https://github.com/brimsec/zq-sample-data $(@D)

sampledata: $(SAMPLEDATA)

bin/$(ZEEKPATH):
	@mkdir -p bin
	@curl -L -o bin/$(ZEEKPATH).zip \
		https://github.com/brimsec/zeek/releases/download/$(ZEEKTAG)/zeek-$(ZEEKTAG).$$(go env GOOS)-$$(go env GOARCH).zip
	@unzip -q bin/$(ZEEKPATH).zip -d bin \
		&& mv bin/zeek bin/$(ZEEKPATH)

bin/$(SURICATAPATH):
	@mkdir -p bin
	curl -L -o bin/$(SURICATAPATH).zip \
		https://github.com/brimsec/build-suricata/releases/download/$(SURICATATAG)/suricata-$(SURICATATAG).$$(go env GOOS)-$$(go env GOARCH).zip
	unzip -q bin/$(SURICATAPATH).zip -d bin \
		&& mv bin/suricata bin/$(SURICATAPATH)

.PHONY: bin/go.mod
bin/go.mod:
	@rm -rf bin
	@mkdir -p bin
	@cd bin && go mod init deps

bin/minio: bin/go.mod
	@mkdir -p bin
	cd bin && go get -d github.com/minio/minio@1341bf5a9e284c1764baf08c1ee7d2a6af0b4a02
	cd bin && go mod edit -replace 'github.com/minio/minio=github.com/brimsec/minio@v0.0.0-20201019191454-3c6f24527f6d'
	cd bin && go mod download github.com/minio/minio
	cd bin && GOBIN="$(CURDIR)/bin" go install github.com/minio/minio

			#
generate:
	@GOBIN="$(CURDIR)/bin" go install github.com/golang/mock/mockgen
	@PATH="$(CURDIR)/bin:$(PATH)" go generate ./...

test-generate: generate
	git diff --exit-code

test-unit:
	@go test -short ./...

test-system: build bin/minio bin/$(ZEEKPATH) bin/$(SURICATAPATH)
	@ZTEST_PATH="$(CURDIR)/dist:$(CURDIR)/bin:$(CURDIR)/bin/$(ZEEKPATH):$(CURDIR)/bin/$(SURICATAPATH)" go test -v .

test-run: build bin/minio bin/$(ZEEKPATH) bin/$(SURICATAPATH)
	@ZTEST_PATH="$(CURDIR)/dist:$(CURDIR)/bin:$(CURDIR)/bin/$(ZEEKPATH):$(CURDIR)/bin/$(SURICATAPATH)" go test -v . -run $(TEST)

test-heavy: build $(SAMPLEDATA)
	@go test -v -tags=heavy ./tests

test-pcapingest: bin/$(ZEEKPATH)
	@ZEEK="$(CURDIR)/bin/$(ZEEKPATH)/zeekrunner" go test -v -run=PcapPost -tags=pcapingest ./ppl/zqd

perf-compare: build $(SAMPLEDATA)
	scripts/comparison-test.sh

zng-output-check: build $(SAMPLEDATA)
	scripts/zng-output-check.sh

# If the build recipe changes, please also change npm/build.
build: $(PEG_DEP)
	@mkdir -p dist
	@go build -ldflags='$(LDFLAGS)' -o dist ./cmd/... ./ppl/cmd/...

install:
	@go install -ldflags='$(LDFLAGS)' ./cmd/... ./ppl/cmd/...

docker:
	DOCKER_BUILDKIT=1 docker build --pull --rm \
		--build-arg LDFLAGS='$(LDFLAGS)' \
		-t zqd:latest \
		.

docker-push-local: docker
	docker tag zqd localhost:5000/zqd:latest
	docker push localhost:5000/zqd:latest
	docker tag zqd localhost:5000/zqd:$(VERSION)
	docker push localhost:5000/zqd:$(VERSION)

docker-push-ecr: docker
	aws ecr get-login-password --region us-east-2 | docker login \
	  --username AWS --password-stdin $(ZQD_ECR_HOST)/zqd
	docker tag zqd $(ZQD_ECR_HOST)/zqd:$(ECR_VERSION)
	docker push $(ZQD_ECR_HOST)/zqd:$(ECR_VERSION)

kubectl-config:
	kubectl create namespace $(ZQD_K8S_USER)
	kubectl config set-context zqtest \
	--namespace=$(ZQD_K8S_USER) \
	--cluster=$(ZQD_TEST_CLUSTER) \
	--user=$(ZQD_K8S_USER)@$(ZQD_TEST_CLUSTER)
	kubectl config use-context zqtest

helm-install:
	helm install zqd charts/zqd \
	--set AWSRegion="us-east-2" \
	--set image.repository="$(ZQD_ECR_HOST)/" \
	--set image.tag="zqd:$(ECR_VERSION)" \
	--set useCredSecret=false \
	--set datauri=$(ZQD_DATA_URI)

create-release-assets:
	for os in darwin linux windows; do \
		zqdir=zq-$(VERSION).$${os}-amd64 ; \
		rm -rf dist/$${zqdir} ; \
		mkdir -p dist/$${zqdir} ; \
		cp LICENSE.txt acknowledgments.txt dist/$${zqdir} ; \
		GOOS=$${os} GOARCH=amd64 go build -ldflags='$(LDFLAGS)' -o dist/$${zqdir} ./cmd/... ./ppl/cmd/... ; \
	done
	rm -rf dist/release && mkdir -p dist/release
	cd dist && for d in zq-$(VERSION)* ; do \
		zip -r release/$${d}.zip $${d} ; \
	done

build-python-wheel: build-python-lib
	pip3 wheel --no-deps -w dist python/brim

build-python-lib:
	@mkdir -p python/brim/build/zqext
	go build -buildmode=c-archive -o python/brim/build/zqext/libzqext.a python/brim/src/zqext.go

clean-python:
	@rm -rf python/brim/build

PEG_GEN = zql/zql.go zql/zql.js zql/zql.es.js
$(PEG_GEN): zql/Makefile zql/parser-support.js zql/zql.peg
	$(MAKE) -C zql

# This rule is best for edit-compile-debug cycle of peg development.  It should
# properly trigger rebuilds of peg-generated code, but best to run "make" in the
# zql subdirectory if you are changing versions of pigeon, pegjs, or javascript
# dependencies.
.PHONY: peg peg-run
peg: $(PEG_GEN)

peg-run: $(PEG_GEN)
	go run ./cmd/ast -repl

# CI performs these actions individually since that looks nicer in the UI;
# this is a shortcut so that a local dev can easily run everything.
test-ci: fmt tidy vet test-generate test-unit test-system test-pcapingest test-heavy

clean: clean-python
	@rm -rf dist

.PHONY: fmt tidy vet test-unit test-system test-heavy sampledata test-ci
.PHONY: perf-compare build install create-release-assets clean clean-python
.PHONY: build-python-wheel generate test-generate bin/minio
