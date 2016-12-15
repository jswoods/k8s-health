FROM scratch
MAINTAINER Jestin Woods
MAINTAINER Mikhail Khodorovskiy

ADD healthcheck /healthcheck
EXPOSE 8080/tcp

ENTRYPOINT ["/healthcheck"]
