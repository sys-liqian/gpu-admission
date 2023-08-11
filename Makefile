REGISTRY ?= localhost:5000/test
CI_TAG ?= 1.23.17
GOOS = linux
GOARCH = amd64

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: build
build: fmt
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -o bin/gpu-admission

.PHONY: img
img: build
	docker build -t $(REGISTRY)/gpu-admission:$(CI_TAG) -f Dockerfile bin
	# docker push $(REGISTRY)/gpu-admission:$(CI_TAG)
	# docker rmi $(REGISTRY)/gpu-admission:$(CI_TAG)

.PHONY: deploy
deploy:
	kubectl apply -f deploy/

.PHONY: undeploy
undeploy:
	kubectl delete -f deploy/