package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// https://stackoverflow.com/a/16491396
type Config struct {
	APIKey string
}

const namespace = "nagios"
const apiSlug = "/api/v1"
const hoststatusAPI = apiSlug + "/objects/hoststatus"
const servicestatusAPI = apiSlug + "/objects/servicestatus"
const systeminfoAPI = apiSlug + "/system/info"

func ReadConfig(configPath string) Config {

	var conf Config
	if _, err := toml.DecodeFile(configPath, &conf); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	return conf
}

func main() {

	var (
		listenAddress = flag.String("web.listen-address", ":9111",
			"Address to listen on for telemetry")
		metricsPath = flag.String("web.telemetry-path", "/metrics",
			"Path under which to expose metrics")
		remoteAddress = flag.String("web.remote-address", "localhost",
			"Nagios application address")
		configPath = flag.String("config.path", "/etc/nagios_exporter/config.toml",
			"Config file path")
	)

	flag.Parse()

	var conf Config = ReadConfig(*configPath)

	// Metrics
	// TODO - double check I'm naming these metrics right .. like they all have _total?
	// Hosts
	hostsTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_total"),
		"Amount of hosts present in configuration",
		nil, nil,
	)

	hostsActivelyCheckedTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_actively_checked_total"),
		"Amount of hosts actively checked",
		nil, nil,
	)

	hostsPassiveCheckedTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_passively_checked_total"),
		"Amount of hosts passively checked",
		nil, nil,
	)

	hostsUp = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_up_total"),
		"Amount of hosts in 'up' state",
		nil, nil,
	)

	hostsDown = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_down_total"),
		"Amount of hosts in 'down' state",
		nil, nil,
	)

	hostsUnreachable = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_unreachable_total"),
		"Amount of hosts in 'unreachable' state",
		nil, nil,
	)

	hostsFlapping = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_flapping_total"),
		"Amount of hosts in 'flapping' state",
		nil, nil,
	)

	hostsDowntime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "hosts_downtime_total"),
		"Amount of hosts in downtime",
		nil, nil,
	)

	// Services

	servicesTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_total"),
		"Amount of services present in configuration",
		nil, nil,
	)

	servicesActivelyCheckedTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_actively_checked_total"),
		"Amount of services actively checked",
		nil, nil,
	)

	servicesPassiveCheckedTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_passively_checked_total"),
		"Amount of services passively checked",
		nil, nil,
	)

	servicesUp = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_up_total"),
		"Amount of services in 'up' state",
		nil, nil,
	)

	servicesDown = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_down_total"),
		"Amount of services in 'down' state",
		nil, nil,
	)

	servicesUnreachable = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_unreachable_total"),
		"Amount of services in 'unreachable' state",
		nil, nil,
	)

	servicesFlapping = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_flapping_total"),
		"Amount of services in 'flapping' state",
		nil, nil,
	)

	servicesDowntime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_downtime_total"),
		"Amount of services in downtime",
		nil, nil,
	)

	// System
	versionInfo = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "version_info"),
		"Nagios version information",
		nil, nil,
	)

	// TODO - HTTPS?
	nagiosURL := "http://" + *remoteAddress + "/nagiosxi/api/v1/objects/servicestatus?apikey=" + conf.APIKey

	req, err := http.NewRequest("GET", nagiosURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// TODO - assign type
	var j interface{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		panic(err)
	}
	// fmt.Printf("%s", j)

	

	http.Handle(*metricsPath, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
