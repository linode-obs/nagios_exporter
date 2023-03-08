# taken from https://github.com/prometheus/haproxy_exporter/blob/d4aba878f043fd3ad0bcacd0149e7d75e67c0faa/Dockerfile
ARG ARCH="amd64"
ARG OS="linux"
# they don't tag versions only latest
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
# https://github.com/prometheus/busybox
LABEL maintainer="Will Bollock <wbollock@gmail.com>"

COPY nagios_exporter /bin/nagios_exporter

EXPOSE      9927
USER        nobody
ENTRYPOINT  [ "/bin/nagios_exporter" ]
