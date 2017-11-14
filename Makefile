.PHONY: clean install run

default: run

run: build
	@./unusedarg go.avalanche.space/lyft

build:
	@go build -i ./...

clean:
	@rm -rf ./unusedarg
