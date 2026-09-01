.PHONY: test test-go test-python test-plugin test-ui test-contract test-e2e

POSTGRES_TEST_DSN ?= postgres://stash_recommendations:stash_recommendations@127.0.0.1:5432/stash_recommendations?sslmode=disable

test: test-go test-python test-ui

test-go:
	POSTGRES_TEST_DSN=$(POSTGRES_TEST_DSN) go test ./server/...

test-python:
	PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q

test-plugin:
	PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q

test-ui:
	node --test tests/ui/*.test.js

test-contract:
	POSTGRES_TEST_DSN=$(POSTGRES_TEST_DSN) go test ./server/internal/httpapi -run 'TestPreferenceEventFixturesPostThroughHTTPAPI|TestSourceSnapshotFixturesPostThroughHTTPAPI' -v
	PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_contract_fixtures.py -q

test-e2e:
	POSTGRES_TEST_DSN=$(POSTGRES_TEST_DSN) PYTHONPATH=plugin/stashRecommendations pytest tests/e2e -q
