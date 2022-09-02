# nagiosxi_exporter

## Build and Release Steps

1. Build binaries with goreleaser:

```bash
goreleaser build --snapshot --rm-dist
```

2. Use the resulting binaries in `./dist`, or create a deb/rpm packages with nfpm:

```bash
# deb example - can substitute with rpm
nfpm package -p deb -t /tmp/
```

3. Tag release and push:

```
git tag -a v0.1.0 -m "First release"
git push origin v0.1.0
goreleaser release
```


## Resources

* [haproxy_expoter](https://github.com/prometheus/haproxy_exporter/blob/main/haproxy_exporter.go)
* [15 Steps to Write an Application Prometheus Exporter in GO](https://medium.com/teamzerolabs/15-steps-to-write-an-application-prometheus-exporter-in-go-9746b4520e26)
* [curl-to-go](https://mholt.github.io/curl-to-go/)
* [mirth_exporter](https://github.com/teamzerolabs/mirth_channel_exporter)
* [golang-json-api-client](https://blog.alexellis.io/golang-json-api-client/)
* [jsonutils](https://github.com/bashtian/jsonutils)
* [goreleaser](https://github.com/goreleaser/goreleaser)
* [nfpm](https://github.com/goreleaser/nfpm)
