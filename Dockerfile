FROM scratch
MAINTAINER Mikhail Khodorovskiy

ADD healthcheck /healthcheck
EXPOSE 8080/tcp

ENTRYPOINT ["/healthcheck"]
