FROM scratch
MAINTAINER Jestin Woods

ADD healthcheck /healthcheck
EXPOSE 8080/tcp

ENTRYPOINT ["/healthcheck"]
