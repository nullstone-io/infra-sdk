NAME := infra-sdk

.PHONY: test

.DEFAULT_GOAL: default

default: test

test:
	go fmt ./...
	gotestsum --format=testname

update-platforms: update-aws update-gcp update-azure tidy

update-aws:
	go get -u $$(go list -mod=mod -m all | grep '^github.com/aws/aws-sdk-go-v2' | awk '{print $$1}')

update-gcp:
	go get -u $$(go list -mod=mod -m all | grep '^cloud.google.com/go' | awk '{print $$1}')

update-azure:
	go get -u $$(go list -mod=mod -m all | grep '^github.com/Azure/azure-sdk-for-go' | awk '{print $$1}')

tidy:
	go mod tidy
