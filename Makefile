.PHONY: clean install run

default: run

run: build
	@./unusedarg ~/go/src/go.avalanche.space/lyft-go/...

build:
	@go build -i ./...

clean:
	@rm ./unusedarg
