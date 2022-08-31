package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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
const nagiosAPIVersion = "/nagiosxi"
const apiSlug = "/api/v1"
const hoststatusAPI = "/objects/hoststatus"
const servicestatusAPI = "/objects/servicestatus"
const systeminfoAPI = "/system/info"
const systemstatusAPI = "/system/status"

type systemStatus struct {
	// https://stackoverflow.com/questions/21151765/cannot-unmarshal-string-into-go-value-of-type-int64
	Running float64 `json:"is_currently_running,string"`
}


type systemInfo struct {
	Version      float64  `json:"version,string"`
}

// generated with https://github.com/bashtian/jsonutils
type hostStatus struct {
	Recordcount int64 `json:"recordcount"`
	Hoststatus []struct {
		HostObjectID               float64  `json:"host_object_id,string"`
		CheckType                  float64  `json:"check_type,string"`
		CurrentState               float64  `json:"current_state,string"`
		IsFlapping                 float64  `json:"is_flapping,string"`
		ScheduledDowntimeDepth     float64  `json:"scheduled_downtime_depth,string"`
	} `json:"hoststatus"`
}

type serviceStatus struct {
	Recordcount   int64 `json:"recordcount"`
	Servicestatus []struct {
		HasBeenChecked             float64  `json:"has_been_checked,string"`
		ShouldBeScheduled          float64  `json:"should_be_scheduled,string"`
		CheckType                  float64  `json:"check_type,string"`
		CurrentState               float64  `json:"current_state,string"`
		IsFlapping                 float64  `json:"is_flapping,string"`
		ScheduledDowntimeDepth     float64  `json:"scheduled_downtime_depth,string"`
	} `json:"servicestatus"`
}


func ReadConfig(configPath string) Config {

	var conf Config
	if _, err := toml.DecodeFile(configPath, &conf); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	return conf
}

var (
	// Metrics
	// TODO - writing in this style seems more readable https://github.com/prometheus/haproxy_exporter/blob/main/haproxy_exporter.go#L138
	// TODO - double check I'm naming these metrics right .. like they all have _total?
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether Nagios can be reached",
		nil, nil,
	)

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
)

type Exporter struct {
	nagiosEndpoint, nagiosAPIKey string
}

func NewExporter(nagiosEndpoint, nagiosAPIKey string) *Exporter {
	return &Exporter{
		nagiosEndpoint: nagiosEndpoint,
		nagiosAPIKey:   nagiosAPIKey,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	// Hosts
	ch <- hostsTotal
	ch <- hostsActivelyCheckedTotal
	ch <- hostsPassiveCheckedTotal
	ch <- hostsUp
	ch <- hostsDown
	ch <- hostsUnreachable
	ch <- hostsFlapping
	ch <- hostsDowntime
	// Services
	ch <- servicesTotal
	ch <- servicesActivelyCheckedTotal
	ch <- servicesPassiveCheckedTotal
	ch <- servicesUp
	ch <- servicesDown
	ch <- servicesUnreachable
	ch <- servicesFlapping
	ch <- servicesDowntime
	// System
	ch <- versionInfo
}

func (e *Exporter) TestNagiosConnectivity() (float64, error) {

	req, err := http.NewRequest("GET", e.nagiosEndpoint+systemstatusAPI+"?apikey="+e.nagiosAPIKey, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Prometheus")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Fatal(readErr)
	}
	// TODO - better logging and error handling here
	systemStatusObject := systemStatus{}
	jsonErr := json.Unmarshal(body, &systemStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	fmt.Println(systemStatusObject.Running)
	// TODO - figure out which err to return and handle scrape failure better
	return systemStatusObject.Running, err
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	nagiosStatus, err := e.TestNagiosConnectivity()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, nagiosStatus,
		)
		log.Println(err)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, nagiosStatus,
	)

	e.HitNagiosRestApisAndUpdateMetrics(ch)

}

func (e *Exporter) HitNagiosRestApisAndUpdateMetrics(ch chan<- prometheus.Metric) {

	// get system version info
	req, err := http.NewRequest("GET", e.nagiosEndpoint+systeminfoAPI+"?apikey="+e.nagiosAPIKey, nil)

	// todo - better error handling on here, much function-ize the calls?
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Prometheus")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Fatal(readErr)
	}
	// TODO - better logging and error handling here
	systemInfoObject := systemInfo{}
	jsonErr := json.Unmarshal(body, &systemInfoObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	// 2022/08/30 20:55:59 json: cannot unmarshal number 5.8.10 into Go struct field systemInfo.version of type float64

	ch <- prometheus.MustNewConstMetric(
		versionInfo, prometheus.GaugeValue, systemInfoObject.Version,
	)

	log.Println("Endpoint scraped")
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

	// TODO - HTTPS?
	nagiosURL := "http://" + *remoteAddress + nagiosAPIVersion + apiSlug
	// nagiosURL := "http://" + *remoteAddress + "/nagiosxi/api/v1/objects/servicestatus?apikey=" + conf.APIKey

	exporter := NewExporter(nagiosURL, conf.APIKey)
	prometheus.MustRegister(exporter)
	// todo - use better logging system
	log.Printf("Using connection endpoint: %s", *remoteAddress)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Nagios Exporter</title></head>
			<body>
			<h1>Nagios Exporter</h1>
			<p><a href='` + *metricsPath + `'>Metrics</a></p>
			</body>
			</html>`))
	})

	log.Fatal(http.ListenAndServe(*listenAddress, nil))

}
