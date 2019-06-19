#sample usage:
#after modify idl:   make kitegen
#to build :  make build
#to build & prepare for local run: make output
#to run this service locally:     make output ;   ./output/bootstrap.sh output
all:
	make build

fmt:
	find . -name "*.go" | grep -v "vendor" | grep -v "clients" | grep -v "pb.go" | xargs goimports -w
	find . -name "*.go" | grep -v "vendor" | grep -v "clients" | grep -v "pb.go" | xargs gofmt -w

format: clean fmt

build:
	./build.sh

clean:
	rm -rf ./output
	rm -f ./tikv_proxy
