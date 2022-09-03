# nagios_exporter

[![golangci-lint](https://github.com/wbollock/nagios_exporter/actions/workflows/golangci-lint.yaml/badge.svg)](https://github.com/wbollock/nagios_exporter/actions/workflows/golangci-lint.yaml)
![Go Report Card](https://goreportcard.com/badge/github.com/wbollock/nagios_exporter)

A Prometheus exporter currently supporting:

* Nagios XI

It includes metrics on the current state and configuration of Nagios. This includes the number of hosts, services, and information about their monitoring setup. For example, this exporter will output the number of flapping hosts, passive checks, or hosts in downtime.

Practical use cases for this exporter may include:

* A Nagios overview - see broad metrics on hosts and services
* Visualize changes in host status after making adjustments to Nagios checks
* Detect an uptick in `unknown` check results after converting many active checks to passive

This exporter does not output Nagios check results as Prometheus metrics; it is designed to export metrics of the Nagios monitoring server itself.

## Installation Instructions

### Configuration

Create a simple `config.toml` in `/etc/prometheus-nagios-exporter` with your Nagios API key:

```toml
# prometheus-nagios-exporter configuration

APIKey = ""
```

By default this will point to localhost, but a remote address can be specified with `--web.remote-address`. The default port is `9111`, but can be changed with `--web.listen-address`.

To see all available configuration flags:

```bash
./prometheus-nagios-exporter -h
```

### Debian/RPM package

Substitute `{{ version }}` for your desired release.

```bash
wget https://github.com/wbollock/nagios_exporter/releases/download/v{{ version }}/prometheus-nagios-exporter_{{ version }}_linux_amd64.deb
dpkg -i prometheus-nagios-exporter_{{ version }}_linux_amd64.deb
```

### Binary

```bash
wget https://github.com/wbollock/nagios_exporter/releases/download/v{{ version }}/nagios_exporter_{{ version }}_Linux_x86_64.tar.gz 
tar xvf nagios_exporter_{{ version }}_Linux_x86_64.tar.gz
./nagios_exporter/prometheus-nagios-exporter
```

### Source

```bash
wget https://github.com/wbollock/nagios_exporter/archive/refs/tags/v{{ version }}.tar.gz
tar xvf nagios_exporter-{{ version }}.tar.gz
cd ./nagios_exporter-{{ version }}
go build main.go
./main.go
```

## Troubleshooting

Ensure `nagios_up` returns `1`, otherwise please check your API key and Nagios reachability, such as:

```bash
curl -GET "http://<nagios_url>/nagiosxi/api/v1/objects/host?apikey=<apikey>&pretty=1"
```

## Grafana

Import the [dashboard](grafana/dashboard.json) template ([instructions](https://grafana.com/docs/grafana/v9.0/dashboards/export-import/#import-dashboard)).

 ![grafana](img/grafana.png)

## Resources Used

* [haproxy_expoter](https://github.com/prometheus/haproxy_exporter/blob/main/haproxy_exporter.go)
* [15 Steps to Write an Application Prometheus Exporter in GO](https://medium.com/teamzerolabs/15-steps-to-write-an-application-prometheus-exporter-in-go-9746b4520e26)
* [curl-to-go](https://mholt.github.io/curl-to-go/)
* [mirth_exporter](https://github.com/teamzerolabs/mirth_channel_exporter)
* [golang-json-api-client](https://blog.alexellis.io/golang-json-api-client/)
* [jsonutils](https://github.com/bashtian/jsonutils)
* [goreleaser](https://github.com/goreleaser/goreleaser)
* [nfpm](https://github.com/goreleaser/nfpm)
