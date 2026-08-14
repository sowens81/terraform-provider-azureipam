TEST?=$$(go list ./... | grep -v 'vendor')
HOSTNAME=registry.terraform.io
NAMESPACE=sowens81
NAME=azureipam
BINARY=terraform-provider-${NAME}
VERSION=2.0.0
OS_ARCH?=$(shell go env GOOS)_$(shell go env GOARCH)
INSTALL_DIR=${HOME}/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

default: install

clean:
	rm -rf dist/${BINARY}

build: clean
	go build -o dist/${BINARY} -ldflags="-X 'main.Version=v${VERSION}'"

release:
	goreleaser release --clean --snapshot --skip=sign,publish

install: build
	mkdir -p ${INSTALL_DIR}
	cp dist/${BINARY} ${INSTALL_DIR}/${BINARY}

test:
	go test -i $(TEST) || exit 1
	echo $(TEST) | xargs -t -n4 go test $(TESTARGS) -timeout=30s -parallel=4

testacc:
	TF_ACC=1 go test $(TEST) -v -cover $(TESTARGS) -timeout 120m
