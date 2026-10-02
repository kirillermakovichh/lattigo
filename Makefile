export GOCACHE := $(CURDIR)/.cache/go-build

.PHONY: check build docs-check
check: docs-check
	python3 scripts/check.py

build:
	python3 scripts/check.py --build

docs-check:
	python3 scripts/check_docs.py
