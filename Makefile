.PHONY: test test-go test-python test-ui

test: test-go test-python test-ui

test-go:
	go test ./server/...

test-python:
	python -m pytest plugin/stashRecommendations/tests

test-ui:
	node --test tests/ui/*.test.js
