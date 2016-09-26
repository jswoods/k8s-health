The k8s-health service is a go app that is deployed to a kubernetes cluster and will collect metrics such as running pods, replication controllers and deployments from the API server and send them to datadog.

## How to Build



```
export GOPATH=<project_root>
# godep restore
make
```

## Build Docker Image

```
docker build -t <k8s-health-image-tag>:<version> .
```

## Publish docker Image

docker push <k8s-health-image-tag>:<version>

## Run Docker Image Locally

Kubernetes mounts common cluster secrets into '/var/run/secrets/kubernetes.io/serviceaccount/' directory in every running pod
The directory contains ca.crt which is the trust chain file and token which is a service token that can be used to authorize API calls
In addition, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT environment variables that are also passed-in into every pod specify the API server IP and port


```
make && docker run -it -e KUBERNETES_SERVICE_HOST=<api_server_url> -e KUBERNETES_SERVICE_PORT=443 -e STATSD_HOST=localhost -e STATSD_PORT=8125 -v <location_of_token_and_cacrt_file_directory>:/var/run/secrets/kubernetes.io/serviceaccount/:ro `docker build -q . `
```

## Run As Deployment in kubernetes

```
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    name: k8s-health
  name: k8s-health
  namespace: default
spec:
  replicas: 1
  template:
    metadata:
      labels:
        name: k8s-health
    spec:
      containers:
      - env:
        - name: STATSD_HOST
          value: dd-agent.default
        - name: STATSD_PORT
          value: "8125"
        - name: TAGS
          value: k8s_health_tag
        image: <k8s-health-image-tag>:<version>
        imagePullPolicy: Always
        name: k8s-health
        terminationMessagePath: /dev/termination-log
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
```
