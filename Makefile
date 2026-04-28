GO ?= go

.PHONY: test fmt tidy

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy
