export GOCACHE := $(CURDIR)/.cache/go-build

.PHONY: check build docs-check
check: docs-check
	python3 scripts/check.py

build:
	python3 scripts/check.py --build

docs-check:
	python3 scripts/check_docs.py

.PHONY: binaries release release-all
VERSION ?= local
binaries:
	go build -o bin/blnode ./cmd/blnode
	go build -o bin/blwallet ./cmd/blwallet
	go build -o bin/blctl ./cmd/blctl

release:
	python3 scripts/release.py --version $(VERSION)

release-all:
	python3 scripts/release.py --version $(VERSION) --all

.PHONY: check-faults
check-faults:
	python3 scripts/branch_coverage.py --faults --race --network --strict
