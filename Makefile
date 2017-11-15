.PHONY: clean build run

default: run

run: build
	@./unusedargs ~/go/src/go.avalanche.space/lyft-go/...

build:
	@go build -i ./...

clean:
	@rm ./unusedargs
