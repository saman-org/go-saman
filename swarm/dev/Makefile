.PHONY: build cluster test

default: build

build:
	go build -o bin/swarm    github.com/saman-org/go-saman/cmd/swarm
	go build -o bin/geth     github.com/saman-org/go-saman/cmd/geth
	go build -o bin/bootnode github.com/saman-org/go-saman/cmd/bootnode

cluster: build
	scripts/boot-cluster.sh

test:
	go test -v github.com/saman-org/go-saman/swarm/...
