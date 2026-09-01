.PHONY: test test-go test-python test-plugin test-ui

test: test-go test-python test-ui

test-go:
	go test ./server/...

test-python:
	PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q

test-plugin:
	PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q

test-ui:
	node --test tests/ui/*.test.js
