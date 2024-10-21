TARGET = ./cc-metric-store
VERSION = 1.3.0
GIT_HASH := $(shell git rev-parse --short HEAD || echo 'development')
CURRENT_TIME = $(shell date +"%Y-%m-%d:T%H:%M:%S")
LD_FLAGS = '-s -X main.date=${CURRENT_TIME} -X main.version=${VERSION} -X main.commit=${GIT_HASH}'

.PHONY: clean test tags swagger $(TARGET)

.NOTPARALLEL:

$(TARGET):
	$(info ===>  BUILD cc-metric-store)
	@go build -ldflags=${LD_FLAGS} ./cmd/cc-metric-store

swagger:
	$(info ===>  GENERATE swagger)
	@go run github.com/swaggo/swag/cmd/swag init -d ./internal/api,./internal/util -g api.go -o ./api
	@mv ./api/docs.go ./internal/api/docs.go

clean:
	$(info ===>  CLEAN)
	@go clean
	@rm -f $(TARGET)

test:
	$(info ===>  TESTING)
	@go clean -testcache
	@go build ./...
	@go vet ./...
	@go test ./...
