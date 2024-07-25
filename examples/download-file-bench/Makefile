
default: local

local:
	@echo "Building server for local OS and CPU architecture"
	CGO_ENABLED=0 go build -o download-file-bench main.go
	@echo "Build server success!"

linux:
	@echo "Building server for OS linux and CPU architecture amd64"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o download-file-bench main.go
	@echo "Build server success!"

arm64:
	@echo "Building server for OS linux and CPU architecture arm64"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o download-file-bench main.go
	@echo "Build server success!"

clean:
	@rm -rf ./download-file-bench

dockerbuild: linux
	docker build -t download-file-bench:1.0.0 .

