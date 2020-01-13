#sample usage:
#after modify idl:   make kitegen
#to build :  make build
#to build & prepare for local run: make output
#to run this service locally:     make output ;   ./output/bootstrap.sh output
all:
	make build tool

fmt:
	find . -name "*.go" | grep -v "vendor" | grep -v "clients" | grep -v "pb.go" | xargs goimports -w
	find . -name "*.go" | grep -v "vendor" | grep -v "clients" | grep -v "pb.go" | xargs gofmt -w

format: clean fmt

build:
	./build.sh

tool:
	mkdir -p output/tools
	go build -v -o scan_tool tools/scan_tool.go
	go build -v -o del_tool tools/del_tool.go
	mv scan_tool output/tools
	mv del_tool output/tools
	chmod +x output/tools/*

clean:
	rm -rf ./output
	rm -f ./tikv_proxy
