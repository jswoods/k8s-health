The k8s-health service is a go app that is deployed to a kubernetes cluster and will collect metrics such as running pods, replication controllers and deployments from the API server and send them to datadog.

## How to Build

```
export GOPATH=<project_root>
godep restore
make build
```

## Publish Docker Image

make push

## Run Docker Image Locally

Kubernetes mounts common cluster secrets into '/var/run/secrets/kubernetes.io/serviceaccount/' directory in every running pod
The directory contains ca.crt which is the trust chain file and token which is a service token that can be used to authorize API calls
In addition, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT environment variables that are also passed-in into every pod specify the API server IP and port


```
make build && docker run -it -e KUBERNETES_SERVICE_HOST=<api_server_url> -e KUBERNETES_SERVICE_PORT=443 -e STATSD_HOST=localhost -e STATSD_PORT=8125 -v <location_of_token_and_cacrt_file_directory>:/var/run/secrets/kubernetes.io/serviceaccount/:ro `docker build -q . `
```

## Run As Deployment in kubernetes

```
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: k8s-health
  namespace: kube-system
  labels:
    name: k8s-health
spec:
  replicas: 1
  template:
    metadata:
      labels:
        name: k8s-health
    spec:
      containers:
      - name: k8s-health
        image: jswoods/k8s-health:1.2.4
        ports:
          - containerPort: 8080
            name: k8shealthtcp
            protocol: TCP
        env:
          - name: STATSD_HOST
            value: "dd-agent.kube-system"
          - name: STATSD_PORT
            value: "8125"
        livenessProbe:
          httpGet:
            path: /
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 30
          timeoutSeconds: 5
        resources:
          limits:
            cpu: 10m
            memory: 200Mi
          requests:
            cpu: 10m
            memory: 200Mi
        volumeMounts:
          - name: ssl-certs-host
            mountPath: /etc/ssl/certs
      restartPolicy: Always
      volumes:
      - name: ssl-certs-host
        hostPath:
          path: /usr/share/ca-certificates
```
