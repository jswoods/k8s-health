The k8s-health service is a go app that is deployed to a kubernetes cluster and will collect metrics such as running pods, replication controllers and deployments from the API server and send them to datadog.

## How to Build

```
godep restore
make
```

## Build Docker Image

```
docker build .
```

## Run Docker Image

```
docker run -it -e API_SERVER_URL=<https://server> -e API_USERNAME=<username> -e API_PASSWORD=<password> -e STATSD_HOST=<statsd host> -e STATSD_PORT=8125 -it jswoods/k8s-health:latest
```
