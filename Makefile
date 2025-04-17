TARGET = ./cc-metric-store
VAR = ./var/checkpoints/
VERSION = 0.1.1
GIT_HASH := $(shell git rev-parse --short HEAD || echo 'development')
CURRENT_TIME = $(shell date +"%Y-%m-%d:T%H:%M:%S")
LD_FLAGS = '-s -X main.date=${CURRENT_TIME} -X main.version=${VERSION} -X main.commit=${GIT_HASH}'

.PHONY: clean distclean test swagger $(TARGET)

.NOTPARALLEL:

$(TARGET): config.json $(VAR)
	$(info ===>  BUILD cc-metric-store)
	@go build -ldflags=${LD_FLAGS} ./cmd/cc-metric-store

config.json:
	@cp ./configs/config.json config.json

$(VAR):
	@mkdir -p $(VAR)

swagger:
	$(info ===>  GENERATE swagger)
	@go run github.com/swaggo/swag/cmd/swag init -d ./internal/api,./internal/util -g api.go -o ./api
	@mv ./api/docs.go ./internal/api/docs.go

clean:
	$(info ===>  CLEAN)
	@go clean
	@rm -f $(TARGET)

distclean: clean
	@rm -rf ./var
	@rm -f config.json

test:
	$(info ===>  TESTING)
	@go clean -testcache
	@go build ./...
	@go vet ./...
	@go test ./...
