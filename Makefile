.DEFAULT_GOAL := test

GO ?= go
CONFIG ?= application.yaml

.PHONY: all fmt test run migrate-up migrate-down gormgen gormgen-check

# all 执行除 run 以外的检查类目标。gormgen-check 会先跑 gormgen。
# migrate-up / migrate-down 依赖 PostgreSQL，down 会回滚，不放进 all。
all: fmt test gormgen-check

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

run:
	$(GO) run ./cmd/server -config $(CONFIG)

migrate-up:
	$(GO) run ./cmd/migrate -action up

migrate-down:
	$(GO) run ./cmd/migrate -action down

gormgen:
	$(GO) run ./cmd/gormgen

gormgen-check: gormgen
	git diff --exit-code -- internal/model/query
