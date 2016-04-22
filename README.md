The k8s-health service is a go app that is deployed to a kubernetes cluster and will collect metrics such as running pods, replication controllers and deployments from the API server and send them to datadog.

## How to Build

godep restore
go build -o healthcheck .

## Build Docker Image

docker build .
