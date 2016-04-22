FROM scratch
MAINTAINER Jestin Woods

ADD ca-certificates.crt /etc/ssl/certs/
ADD healthcheck /healthcheck
EXPOSE 8080/tcp

ENTRYPOINT ["/healthcheck"]
