.PHONY: verify test vet build status live-proof

verify: test vet build status live-proof

test:
	go test -v -count=1 ./...

vet:
	go vet ./...

build:
	go build -o bin/btc-agent .

status:
	./bin/btc-agent status --config config.yaml

live-proof:
	./bin/btc-agent live-proof --config config.yaml
