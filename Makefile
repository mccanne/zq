export GO111MODULE=on

# If VERSION or LDFLAGS change, please also change
# npm/build.
ARCH = "amd64"
VERSION = $(shell git describe --tags --dirty --always)
LDFLAGS = -s -X github.com/brimdata/zed/cli.Version=$(VERSION)
MINIO_VERSION := 0.0.0-20201211152140-453ab257caf5

# This enables a shortcut to run a single test from the ./ztests suite, e.g.:
#  make TEST=TestZed/ztests/suite/cut/cut
ifneq "$(TEST)" ""
test-one: test-run
endif

# Uncomment this to trigger re-builds of the peg files when the grammar
# is out of date.  We are commenting this out to work around issue #1717.
#PEG_DEP=peg

vet:
	@go vet -composites=false -stdmethods=false ./...

fmt:
	gofmt -s -w .
	git diff --exit-code -- '*.go'

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

SAMPLEDATA:=zed-sample-data/README.md

$(SAMPLEDATA):
	git clone -b edu-data --depth=1 https://github.com/brimdata/zed-sample-data $(@D)

sampledata: $(SAMPLEDATA)

.PHONY: bin/minio
bin/minio: bin/minio-$(MINIO_VERSION)
	ln -fs $(<F) $@

bin/minio-$(MINIO_VERSION):
	mkdir -p $(@D)
	echo 'module deps' > $@.mod
	go mod edit -modfile=$@.mod -replace=github.com/minio/minio=github.com/brimdata/minio@v$(MINIO_VERSION)
	go get -d -modfile=$@.mod github.com/minio/minio
	go build -modfile=$@.mod -o $@ github.com/minio/minio

generate:
	@GOBIN="$(CURDIR)/bin" go install github.com/golang/mock/mockgen
	@PATH="$(CURDIR)/bin:$(PATH)" go generate ./...

test-generate: generate
	git diff --exit-code

test-unit:
	@go test -short ./...

test-system: build bin/minio
	@ZTEST_PATH="$(CURDIR)/dist:$(CURDIR)/bin" go test .

test-run: build bin/minio
	@ZTEST_PATH="$(CURDIR)/dist:$(CURDIR)/bin" go test . -run $(TEST)

test-heavy: build $(SAMPLEDATA)
	@PATH="$(CURDIR)/dist:$(PATH)" go test -tags=heavy ./mdtest

.PHONY: test-services
test-services: build
	@ZTEST_PATH="$(CURDIR)/dist:$(CURDIR)/bin" \
		ZTEST_TAG=services \
		go test -run TestZed/ppl/zqd/ztests/redis .

perf-compare: build $(SAMPLEDATA)
	scripts/perf-compare.sh

output-check: build $(SAMPLEDATA)
	scripts/output-check.sh

# If the build recipe changes, please also change npm/build.
build: $(PEG_DEP)
	@mkdir -p dist
	@go build -ldflags='$(LDFLAGS)' -o dist ./cmd/...

install:
	@go install -ldflags='$(LDFLAGS)' ./cmd/...

create-release-assets:
	for os in darwin linux windows; do \
		zqdir=zq-$(VERSION).$${os}-amd64 ; \
		rm -rf dist/$${zqdir} ; \
		mkdir -p dist/$${zqdir} ; \
		cp LICENSE.txt acknowledgments.txt dist/$${zqdir} ; \
		GOOS=$${os} GOARCH=$(ARCH) go build -ldflags='$(LDFLAGS)' -o dist/$${zqdir} ./cmd/... ; \
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

PEG_GEN := $(addprefix compiler/parser/parser., go js es.js)
$(PEG_GEN): compiler/parser/Makefile compiler/parser/support.js compiler/parser/parser.peg
	$(MAKE) -C compiler/parser

# This rule is best for edit-compile-debug cycle of peg development.  It should
# properly trigger rebuilds of peg-generated code, but best to run "make" in the
# zql subdirectory if you are changing versions of pigeon, pegjs, or javascript
# dependencies.
.PHONY: peg peg-run
peg: $(PEG_GEN)

peg-run: $(PEG_GEN)
	go run ./cmd/zc -repl

# CI performs these actions individually since that looks nicer in the UI;
# this is a shortcut so that a local dev can easily run everything.
test-ci: fmt tidy vet test-generate test-unit test-system test-heavy

clean: clean-python
	@rm -rf dist

.PHONY: fmt tidy vet test-unit test-system test-heavy sampledata test-ci
.PHONY: perf-compare build install create-release-assets clean clean-python
.PHONY: build-python-wheel generate test-generate
