.PHONY: build test lint score

build:
	go build -o archscore ./cmd/archscore

test:
	go test ./...

lint:
	go vet ./...

score: build
	./archscore -root . -config archscore.json
