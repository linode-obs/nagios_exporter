package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// https://stackoverflow.com/a/16491396
type Config struct {
	APIKey string
}

const namespace = "nagios"

// NagiosXI specific API endpoints
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
	Version string `json:"version"`
}

// generated with https://github.com/bashtian/jsonutils
type hostStatus struct {
	Recordcount int64 `json:"recordcount"`
	Hoststatus  []struct {
		HostObjectID           float64 `json:"host_object_id,string"`
		CheckType              float64 `json:"check_type,string"`
		CurrentState           float64 `json:"current_state,string"`
		IsFlapping             float64 `json:"is_flapping,string"`
		ScheduledDowntimeDepth float64 `json:"scheduled_downtime_depth,string"`
	} `json:"hoststatus"`
}

type serviceStatus struct {
	Recordcount   int64 `json:"recordcount"`
	Servicestatus []struct {
		HasBeenChecked         float64 `json:"has_been_checked,string"`
		ShouldBeScheduled      float64 `json:"should_be_scheduled,string"`
		CheckType              float64 `json:"check_type,string"`
		CurrentState           float64 `json:"current_state,string"`
		IsFlapping             float64 `json:"is_flapping,string"`
		ScheduledDowntimeDepth float64 `json:"scheduled_downtime_depth,string"`
	} `json:"servicestatus"`
}

func ReadConfig(configPath string) Config {

	var conf Config

	if _, err := toml.DecodeFile(configPath, &conf); err != nil {
		log.Fatal(os.Stderr, "error: %v\n", err)
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

	hostsPassivelyCheckedTotal = prometheus.NewDesc(
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

	servicesPassivelyCheckedTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_passively_checked_total"),
		"Amount of services passively checked",
		nil, nil,
	)

	servicesOk = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_ok_total"),
		"Amount of services in 'up' state",
		nil, nil,
	)

	servicesWarn = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_warn_total"),
		"Amount of services in 'warn' state",
		nil, nil,
	)

	servicesCritical = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_critical_total"),
		"Amount of services in 'critical' state",
		nil, nil,
	)

	servicesUnknown = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "services_unknown_total"),
		"Amount of services in 'unknown' state",
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
		[]string{"version"}, nil,
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
	ch <- hostsPassivelyCheckedTotal
	ch <- hostsUp
	ch <- hostsDown
	ch <- hostsUnreachable
	ch <- hostsFlapping
	ch <- hostsDowntime
	// Services
	ch <- servicesTotal
	ch <- servicesActivelyCheckedTotal
	ch <- servicesPassivelyCheckedTotal
	ch <- servicesOk
	ch <- servicesWarn
	ch <- servicesCritical
	ch <- servicesUnknown
	ch <- servicesFlapping
	ch <- servicesDowntime
	// System
	ch <- versionInfo
}

func (e *Exporter) TestNagiosConnectivity() float64 {

	systemStatusURL := e.nagiosEndpoint + systemstatusAPI + "?apikey=" + e.nagiosAPIKey

	body := QueryAPIs(systemStatusURL)
	log.Debug("Queried API: ", systemstatusAPI)

	systemStatusObject := systemStatus{}

	jsonErr := json.Unmarshal(body, &systemStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	return systemStatusObject.Running
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	nagiosStatus := e.TestNagiosConnectivity()

	if nagiosStatus == 0 {
		log.Warn("Cannot connect to Nagios endpoint")
	}

	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, nagiosStatus,
	)

	e.QueryAPIsAndUpdateMetrics(ch)

}

func QueryAPIs(url string) (body []byte) {

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		log.Warn(os.Stderr, "error: %v\n", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Prometheus")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Warn(os.Stderr, "error: %v\n", err)
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	} else {
		log.Warn("HTTP response body is nil - check API connectivity")
	}

	body, readErr := ioutil.ReadAll(resp.Body)

	if readErr != nil {
		log.Fatal(readErr)
	}

	return body
}

func (e *Exporter) QueryAPIsAndUpdateMetrics(ch chan<- prometheus.Metric) {

	// get system status
	systeminfoURL := e.nagiosEndpoint + systeminfoAPI + "?apikey=" + e.nagiosAPIKey
	log.Debug("Queried API: ", systeminfoAPI)

	body := QueryAPIs(systeminfoURL)

	// TODO - better logging and error handling here
	systemInfoObject := systemInfo{}
	jsonErr := json.Unmarshal(body, &systemInfoObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	ch <- prometheus.MustNewConstMetric(
		versionInfo, prometheus.GaugeValue, 1, systemInfoObject.Version,
	)

	// host status
	hoststatusURL := e.nagiosEndpoint + systeminfoAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(hoststatusURL)
	log.Debug("Queried API: ", systeminfoAPI)

	hostStatusObject := hostStatus{}

	jsonErr = json.Unmarshal(body, &hostStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	ch <- prometheus.MustNewConstMetric(
		hostsTotal, prometheus.GaugeValue, float64(hostStatusObject.Recordcount),
	)

	var hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount, hostsFlapCount, hostsDowntimeCount int

	// iterate through nested json
	for _, v := range hostStatusObject.Hoststatus {

		// for every hosts
		hostsCount++

		if v.CheckType == 0 {
			hostsActiveCheckCount++
		} else {
			hostsPassiveCheckCount++
		}

		switch currentstate := v.CurrentState; currentstate {
		case 0:
			hostsUpCount++
		case 1:
			hostsDownCount++
		case 2:
			hostsUnreachableCount++
		}

		if v.IsFlapping == 1 {
			hostsFlapCount++
		}

		if v.ScheduledDowntimeDepth == 1 {
			hostsDowntimeCount++
		}
	}

	// TODO - some variable names have S'es and some don't
	ch <- prometheus.MustNewConstMetric(
		hostsActivelyCheckedTotal, prometheus.GaugeValue, float64(hostsActiveCheckCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsPassivelyCheckedTotal, prometheus.GaugeValue, float64(hostsPassiveCheckCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsUp, prometheus.GaugeValue, float64(hostsUpCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsDown, prometheus.GaugeValue, float64(hostsDownCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsUnreachable, prometheus.GaugeValue, float64(hostsUnreachableCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsFlapping, prometheus.GaugeValue, float64(hostsFlapCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsDowntime, prometheus.GaugeValue, float64(hostsDowntimeCount),
	)

	// service status
	servicestatusURL := e.nagiosEndpoint + servicestatusAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(servicestatusURL)
	log.Debug("Queried API: ", servicestatusAPI)

	serviceStatusObject := serviceStatus{}

	jsonErr = json.Unmarshal(body, &serviceStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	ch <- prometheus.MustNewConstMetric(
		servicesTotal, prometheus.GaugeValue, float64(serviceStatusObject.Recordcount),
	)

	var servicesCount, servicessCheckedCount, servicesScheduledCount, servicesActiveCheckCount, servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesCriticalCount, servicesUnknownCount, servicesFlapCount, servicesDowntimeCount int

	for _, v := range serviceStatusObject.Servicestatus {

		servicesCount++

		if v.HasBeenChecked == 0 {
			servicessCheckedCount++
		}

		if v.ShouldBeScheduled == 0 {
			// TODO - is should_be_scheduled different than a services actually being scheduled?
			servicesScheduledCount++
		}

		if v.CheckType == 0 {
			// TODO - I'm a little shaky on check_type -> 1 being passive
			servicesActiveCheckCount++
		} else {
			servicesPassiveCheckCount++
		}

		switch currentstate := v.CurrentState; currentstate {
		// TODO - verify this order, e.g 1/2 are warn/crit
		case 0:
			servicesOkCount++
		case 1:
			servicesWarnCount++
		case 2:
			servicesCriticalCount++
		case 3:
			servicesUnknownCount++
		}

		if v.IsFlapping == 1 {
			servicesFlapCount++
		}

		if v.ScheduledDowntimeDepth == 1 {
			servicesDowntimeCount++
		}
	}

	ch <- prometheus.MustNewConstMetric(
		servicesActivelyCheckedTotal, prometheus.GaugeValue, float64(servicesActiveCheckCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesPassivelyCheckedTotal, prometheus.GaugeValue, float64(hostsPassiveCheckCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesOk, prometheus.GaugeValue, float64(servicesOkCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesWarn, prometheus.GaugeValue, float64(servicesWarnCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesCritical, prometheus.GaugeValue, float64(servicesWarnCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesUnknown, prometheus.GaugeValue, float64(servicesUnknownCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesFlapping, prometheus.GaugeValue, float64(servicesFlapCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesDowntime, prometheus.GaugeValue, float64(servicesDowntimeCount),
	)

	log.Info("Endpoint scraped and metrics updated")
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
		logLevel = flag.String("log.level", "info",
			"Minimum Log level [debug, info]")
	)

	flag.Parse()

	if *logLevel == "debug" {
		log.SetLevel(log.DebugLevel)
		log.Debug("Log level set to debug")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	var conf Config = ReadConfig(*configPath)

	// TODO - HTTPS support
	nagiosURL := "http://" + *remoteAddress + nagiosAPIVersion + apiSlug

	exporter := NewExporter(nagiosURL, conf.APIKey)
	prometheus.MustRegister(exporter)

	log.Info("Using connection endpoint: ", *remoteAddress)

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
