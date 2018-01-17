IMAGE_NAME=jswoods/k8s-health
DOCKER_TAG=${IMAGE_NAME}:1.2.7

default: build

deps:
	go get github.com/PagerDuty/godspeed
	go get k8s.io/kubernetes/pkg/api
	go get k8s.io/kubernetes/pkg/fields
	go get k8s.io/kubernetes/pkg/labels
	go get k8s.io/kubernetes/pkg/client/unversioned
	go get k8s.io/kubernetes/pkg/client/restclient

build: deps
	GODEBUG=netdns=cgo CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w' -o healthcheck .
	docker build -t ${DOCKER_TAG} .

push: build
	docker push ${DOCKER_TAG}
