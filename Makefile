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
