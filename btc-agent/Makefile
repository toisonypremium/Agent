.PHONY: fmt-check test vet build linux-build config-check secrets-check immutable-drills verify

fmt-check:
	test -z "$$(gofmt -l .)"

test:
	go test -count=1 ./...

vet:
	go vet ./...

build:
	go build -trimpath -o bin/agent .

linux-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/agent-linux-amd64 .

config-check: build
	./bin/agent config-check --config config.yaml.example

secrets-check:
	bash scripts/check-secrets.sh

immutable-drills:
	bash deploy/test-immutable-backup.sh
	bash deploy/test-immutable-health-check.sh
	bash deploy/test-immutable-runtime.sh
	bash deploy/test-immutable-runtime-verify.sh
	bash deploy/test-immutable-user-release.sh
	bash deploy/test-immutable-web-console-observer-release.sh
	bash deploy/test-immutable-web-console-okx-assets-observer-release.sh

verify: fmt-check test vet config-check secrets-check immutable-drills
