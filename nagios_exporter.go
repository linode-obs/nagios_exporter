package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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
const systemstatusDetailAPI = "/system/statusdetail"

type systemStatus struct {
	// https://stackoverflow.com/questions/21151765/cannot-unmarshal-string-into-go-value-of-type-int64
	Running float64 `json:"is_currently_running,string"`
}

type systemStatusDetail struct {
	Nagioscore struct {
		Activehostcheckperf struct {
			AvgExecutionTime float64 `json:"avg_execution_time,string"`
			AvgLatency       float64 `json:"avg_latency,string"`
			MaxExecutionTime float64 `json:"max_execution_time,string"`
			MaxLatency       float64 `json:"max_latency,string"`
			MinExecutionTime int64   `json:"min_execution_time,string"`
			MinLatency       int64   `json:"min_latency,string"`
		} `json:"activehostcheckperf"`
		Activehostchecks struct {
			Val1  int64 `json:"val1,string"`
			Val15 int64 `json:"val15,string"`
			Val5  int64 `json:"val5,string"`
		} `json:"activehostchecks"`
		Activeservicecheckperf struct {
			AvgExecutionTime float64 `json:"avg_execution_time,string"`
			AvgLatency       float64 `json:"avg_latency,string"`
			MaxExecutionTime float64 `json:"max_execution_time,string"`
			MaxLatency       float64 `json:"max_latency,string"`
			MinExecutionTime int64   `json:"min_execution_time,string"`
			MinLatency       int64   `json:"min_latency,string"`
		} `json:"activeservicecheckperf"`
		Activeservicechecks struct {
			Val1  int64 `json:"val1,string"`
			Val15 int64 `json:"val15,string"`
			Val5  int64 `json:"val5,string"`
		} `json:"activeservicechecks"`
		Passivehostchecks struct {
			Val1  int64 `json:"val1,string"`
			Val15 int64 `json:"val15,string"`
			Val5  int64 `json:"val5,string"`
		} `json:"passivehostchecks"`
		Passiveservicechecks struct {
			Val1  int64 `json:"val1,string"`
			Val15 int64 `json:"val15,string"`
			Val5  int64 `json:"val5,string"`
		} `json:"passiveservicechecks"`
		Updated string `json:"updated"`
	} `json:"nagioscore"`
}

type systemInfo struct {
	Version string `json:"version"`
}

// generated with https://github.com/bashtian/jsonutils
type hostStatus struct {
	Recordcount int64 `json:"recordcount"`
	Hoststatus  []struct {
		HostObjectID               float64 `json:"host_object_id,string"`
		CheckType                  float64 `json:"check_type,string"`
		CurrentState               float64 `json:"current_state,string"`
		IsFlapping                 float64 `json:"is_flapping,string"`
		ScheduledDowntimeDepth     float64 `json:"scheduled_downtime_depth,string"`
		ProblemHasBeenAcknowledged int64   `json:"problem_has_been_acknowledged,string"`
	} `json:"hoststatus"`
}

type serviceStatus struct {
	Recordcount   int64 `json:"recordcount"`
	Servicestatus []struct {
		HasBeenChecked             float64 `json:"has_been_checked,string"`
		ShouldBeScheduled          float64 `json:"should_be_scheduled,string"`
		CheckType                  float64 `json:"check_type,string"`
		CurrentState               float64 `json:"current_state,string"`
		IsFlapping                 float64 `json:"is_flapping,string"`
		ScheduledDowntimeDepth     float64 `json:"scheduled_downtime_depth,string"`
		ProblemHasBeenAcknowledged int64   `json:"problem_has_been_acknowledged,string"`
	} `json:"servicestatus"`
}

func ReadConfig(configPath string) Config {

	var conf Config

	if _, err := toml.DecodeFile(configPath, &conf); err != nil {
		log.Fatal(err)
	}

	return conf
}

var (
	// Build info for nagios exporter itself, will be populated by linker during build
	Version   string
	BuildDate string
	Commit    string

	// Metrics
	up = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"), "Whether Nagios can be reached", nil, nil)

	// Hosts
	hostsTotal        = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_total"), "Amount of hosts present in configuration", nil, nil)
	hostsCheckedTotal = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_checked_total"), "Amount of hosts checked", []string{"check_type"}, nil)
	hostsStatus       = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_status_total"), "Amount of hosts in different states", []string{"status"}, nil)
	// downtime seems like a separate entity from status
	hostsDowntime = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_downtime_total"), "Amount of hosts in downtime", nil, nil)
	// TODO - maybe it is time to make host/service a label too...
	hostsProblemsAcknowledged = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_acknowledges_total"), "Amount of host problems acknowledged", nil, nil)
	// Services

	servicesTotal                = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "services_total"), "Amount of services present in configuration", nil, nil)
	servicesCheckedTotal         = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "services_checked_total"), "Amount of services checked", []string{"check_type"}, nil)
	servicesStatus               = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "services_status_total"), "Amount of services in different states", []string{"status"}, nil)
	servicesDowntime             = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "services_downtime_total"), "Amount of services in downtime", nil, nil)
	servicesProblemsAcknowledged = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "services_acknowledges_total"), "Amount of service problems acknowledged", nil, nil)

	// System
	versionInfo = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "version_info"), "Nagios version information", []string{"version"}, nil)
	buildInfo   = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "build_info"), "Nagios exporter build information", []string{"version", "build_date", "commit"}, nil)

	// System Detail
	hostchecks    = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "host_checks_minutes"), "Host checks over time", []string{"check_type"}, nil)
	servicechecks = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "service_checks_minutes"), "Service checks over time", []string{"check_type"}, nil)
	// TODO - probably not ideal to have average already baked in, but don't know if I can calculate it myself..
	// feels really nasty to have an operator label, maybe I stick to only making the average a metric?
	// operator is min/max/avg exposed by Nagios XI API
	// performance_type is latency/execution
	// technically there is no such thing as a check_type of passive for these metrics, I guess I still keep the label though
	hostchecksPerformance    = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "host_checks_performance_seconds"), "Host checks performance", []string{"check_type", "performance_type", "operator"}, nil)
	servicechecksPerformance = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "service_checks_performance_seconds"), "Service checks performance", []string{"check_type", "performance_type", "operator"}, nil)
)

type Exporter struct {
	nagiosEndpoint, nagiosAPIKey string
	sslVerify                    bool
	nagiosAPITimeout             time.Duration
}

func NewExporter(nagiosEndpoint, nagiosAPIKey string, sslVerify bool, nagiosAPITimeout time.Duration) *Exporter {
	return &Exporter{
		nagiosEndpoint:   nagiosEndpoint,
		nagiosAPIKey:     nagiosAPIKey,
		sslVerify:        sslVerify,
		nagiosAPITimeout: nagiosAPITimeout,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// Nagios status
	ch <- up
	// Hosts
	ch <- hostsTotal
	ch <- hostsCheckedTotal
	ch <- hostsStatus
	ch <- hostsDowntime
	ch <- hostsProblemsAcknowledged
	// Services
	ch <- servicesTotal
	ch <- servicesCheckedTotal
	ch <- servicesStatus
	ch <- servicesDowntime
	ch <- servicesProblemsAcknowledged
	// System
	ch <- versionInfo
	ch <- buildInfo
	// System Detail
	ch <- hostchecks
	ch <- servicechecks
	ch <- hostchecksPerformance
	ch <- servicechecksPerformance
}

func (e *Exporter) TestNagiosConnectivity(sslVerify bool, nagiosAPITimeout time.Duration) float64 {

	systemStatusURL := e.nagiosEndpoint + systemstatusAPI + "?apikey=" + e.nagiosAPIKey

	body := QueryAPIs(systemStatusURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", systemstatusAPI)

	systemStatusObject := systemStatus{}

	jsonErr := json.Unmarshal(body, &systemStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	return systemStatusObject.Running
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	nagiosStatus := e.TestNagiosConnectivity(e.sslVerify, e.nagiosAPITimeout)

	if nagiosStatus == 0 {
		log.Warn("Cannot connect to Nagios endpoint")
	}

	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, nagiosStatus,
	)

	e.QueryAPIsAndUpdateMetrics(ch, e.sslVerify, e.nagiosAPITimeout)

}

func QueryAPIs(url string, sslVerify bool, nagiosAPITimeout time.Duration) (body []byte) {

	// https://github.com/prometheus/haproxy_exporter/blob/main/haproxy_exporter.go#L337-L345

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: !sslVerify}}

	client := http.Client{
		Timeout:   nagiosAPITimeout,
		Transport: tr,
	}

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		log.Warn(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Prometheus")

	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	} else {
		log.Fatal("HTTP response body is nil - check API connectivity")
	}

	body, readErr := io.ReadAll(resp.Body)

	if readErr != nil {
		log.Fatal(readErr)
	}

	return body
}

func (e *Exporter) QueryAPIsAndUpdateMetrics(ch chan<- prometheus.Metric, sslVerify bool, nagiosAPITimeout time.Duration) {

	// get system status
	systeminfoURL := e.nagiosEndpoint + systeminfoAPI + "?apikey=" + e.nagiosAPIKey
	log.Debug("Queried API: ", systeminfoAPI)

	body := QueryAPIs(systeminfoURL, sslVerify, nagiosAPITimeout)

	systemInfoObject := systemInfo{}
	jsonErr := json.Unmarshal(body, &systemInfoObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	ch <- prometheus.MustNewConstMetric(
		versionInfo, prometheus.GaugeValue, 1, systemInfoObject.Version,
	)

	ch <- prometheus.MustNewConstMetric(
		buildInfo, prometheus.GaugeValue, 1, Version, BuildDate, Commit,
	)

	// host status
	hoststatusURL := e.nagiosEndpoint + hoststatusAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(hoststatusURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", systeminfoAPI)

	hostStatusObject := hostStatus{}

	jsonErr = json.Unmarshal(body, &hostStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	ch <- prometheus.MustNewConstMetric(
		hostsTotal, prometheus.GaugeValue, float64(hostStatusObject.Recordcount),
	)

	var hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount, hostsFlapCount, hostsDowntimeCount, hostsProblemsAcknowledgedCount int

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

		if v.ProblemHasBeenAcknowledged == 1 {
			hostsProblemsAcknowledgedCount++
		}
	}

	ch <- prometheus.MustNewConstMetric(
		hostsCheckedTotal, prometheus.GaugeValue, float64(hostsActiveCheckCount), "active",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsCheckedTotal, prometheus.GaugeValue, float64(hostsPassiveCheckCount), "passive",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, float64(hostsUpCount), "up",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, float64(hostsDownCount), "down",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, float64(hostsUnreachableCount), "unreachable",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, float64(hostsFlapCount), "flapping",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsDowntime, prometheus.GaugeValue, float64(hostsDowntimeCount),
	)

	ch <- prometheus.MustNewConstMetric(
		hostsProblemsAcknowledged, prometheus.GaugeValue, float64(hostsProblemsAcknowledgedCount),
	)

	// service status
	servicestatusURL := e.nagiosEndpoint + servicestatusAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(servicestatusURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", servicestatusAPI)

	serviceStatusObject := serviceStatus{}

	jsonErr = json.Unmarshal(body, &serviceStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	ch <- prometheus.MustNewConstMetric(
		servicesTotal, prometheus.GaugeValue, float64(serviceStatusObject.Recordcount),
	)

	var servicesCount, servicessCheckedCount, servicesScheduledCount, servicesActiveCheckCount,
		servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesCriticalCount,
		servicesUnknownCount, servicesFlapCount, servicesDowntimeCount, servicesProblemsAcknowledgedCount int

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

		if v.ProblemHasBeenAcknowledged == 1 {
			servicesProblemsAcknowledgedCount++
		}
	}

	ch <- prometheus.MustNewConstMetric(
		servicesCheckedTotal, prometheus.GaugeValue, float64(servicesActiveCheckCount), "active",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesCheckedTotal, prometheus.GaugeValue, float64(hostsPassiveCheckCount), "passive",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, float64(servicesOkCount), "ok",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, float64(servicesWarnCount), "warn",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, float64(servicesWarnCount), "critical",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, float64(servicesUnknownCount), "unknown",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, float64(servicesFlapCount), "flapping",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesDowntime, prometheus.GaugeValue, float64(servicesDowntimeCount),
	)

	ch <- prometheus.MustNewConstMetric(
		servicesProblemsAcknowledged, prometheus.GaugeValue, float64(servicesProblemsAcknowledgedCount),
	)

	// service status
	systemStatusDetailURL := e.nagiosEndpoint + systemstatusDetailAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(systemStatusDetailURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", systemstatusDetailAPI)

	systemStatusDetailObject := systemStatusDetail{}

	jsonErr = json.Unmarshal(body, &systemStatusDetailObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	activeHostCheckSum := systemStatusDetailObject.Nagioscore.Activehostchecks.Val1 +
		systemStatusDetailObject.Nagioscore.Activehostchecks.Val5 +
		systemStatusDetailObject.Nagioscore.Activehostchecks.Val15

	ch <- prometheus.MustNewConstHistogram(
		hostchecks, uint64(activeHostCheckSum), float64(activeHostCheckSum), map[float64]uint64{
			1:  uint64(systemStatusDetailObject.Nagioscore.Activehostchecks.Val1),
			5:  uint64(systemStatusDetailObject.Nagioscore.Activehostchecks.Val5),
			15: uint64(systemStatusDetailObject.Nagioscore.Activehostchecks.Val15)}, "active",
	)

	passiveHostCheckSum := systemStatusDetailObject.Nagioscore.Passivehostchecks.Val1 +
		systemStatusDetailObject.Nagioscore.Passivehostchecks.Val5 +
		systemStatusDetailObject.Nagioscore.Passivehostchecks.Val15

	ch <- prometheus.MustNewConstHistogram(
		hostchecks, uint64(passiveHostCheckSum), float64(passiveHostCheckSum), map[float64]uint64{
			1:  uint64(systemStatusDetailObject.Nagioscore.Passivehostchecks.Val1),
			5:  uint64(systemStatusDetailObject.Nagioscore.Passivehostchecks.Val5),
			15: uint64(systemStatusDetailObject.Nagioscore.Passivehostchecks.Val15)}, "passive",
	)

	activeServiceCheckSum := systemStatusDetailObject.Nagioscore.Activeservicechecks.Val1 +
		systemStatusDetailObject.Nagioscore.Activeservicechecks.Val5 +
		systemStatusDetailObject.Nagioscore.Activeservicechecks.Val15

	ch <- prometheus.MustNewConstHistogram(
		servicechecks, uint64(activeServiceCheckSum), float64(activeServiceCheckSum), map[float64]uint64{
			1:  uint64(systemStatusDetailObject.Nagioscore.Activeservicechecks.Val1),
			5:  uint64(systemStatusDetailObject.Nagioscore.Activeservicechecks.Val5),
			15: uint64(systemStatusDetailObject.Nagioscore.Activeservicechecks.Val15)}, "active",
	)

	passiveServiceCheckSum := systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val1 +
		systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val5 +
		systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val15

	ch <- prometheus.MustNewConstHistogram(
		servicechecks, uint64(passiveServiceCheckSum), float64(passiveServiceCheckSum), map[float64]uint64{
			1:  uint64(systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val1),
			5:  uint64(systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val5),
			15: uint64(systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val15)}, "passive",
	)

	// active host check performance
	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activehostcheckperf.AvgLatency), "active", "latency", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activehostcheckperf.MinLatency), "active", "latency", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activehostcheckperf.MaxLatency), "active", "latency", "max",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activehostcheckperf.AvgExecutionTime), "active", "execution", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activehostcheckperf.MinExecutionTime), "active", "execution", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activehostcheckperf.MinExecutionTime), "active", "execution", "max",
	)

	// active service check performance
	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activeservicecheckperf.AvgLatency), "active", "latency", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MinLatency), "active", "latency", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MaxLatency), "active", "latency", "max",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activeservicecheckperf.AvgExecutionTime), "active", "execution", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MinExecutionTime), "active", "execution", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, float64(systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MinExecutionTime), "active", "execution", "max",
	)

	log.Info("Endpoint scraped and metrics updated")
}

// custom formatter modified from https://github.com/sirupsen/logrus/issues/719#issuecomment-536459432
// https://stackoverflow.com/questions/48971780/how-to-change-the-format-of-log-output-in-logrus/48972299#48972299
// required as Nagios XI API only supports giving the API token as a URL parameter, and thus can be leaked in the logs
type nagiosFormatter struct {
	log.TextFormatter
	APIKey string
}

func (f *nagiosFormatter) Format(entry *log.Entry) ([]byte, error) {
	log, newEntry := f.TextFormatter.Format(entry)

	// there might be a better way to do this but, convert our log byte array to a string
	logString := string(log[:])
	// replace the secret APIKey with junk
	cleanString := strings.ReplaceAll(logString, f.APIKey, "<redactedAPIKey>")
	// return it to a byte and pass it on
	cleanLog := []byte(cleanString)

	return bytes.Trim(cleanLog, f.APIKey), newEntry
}

func main() {

	var (
		listenAddress = flag.String("web.listen-address", ":9927",
			"Address to listen on for telemetry")
		metricsPath = flag.String("web.telemetry-path", "/metrics",
			"Path under which to expose metrics")
		remoteAddress = flag.String("nagios.scrape-uri", "http://localhost",
			"Nagios application address")
		sslVerify = flag.Bool("nagios.ssl-verify", false,
			"SSL certificate validation")
		// I think users would rather enter `5` over `5s`, e.g int vs Duration flag
		nagiosAPITimeout = flag.Int("nagios.timeout", 5,
			"Timeout for querying Nagios API in seconds")
		configPath = flag.String("config.path", "/etc/prometheus-nagios-exporter/config.toml",
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

	formatter := nagiosFormatter{}
	formatter.APIKey = conf.APIKey
	log.SetFormatter(&formatter)

	nagiosURL := *remoteAddress + nagiosAPIVersion + apiSlug

	// convert timeout flag to seconds
	exporter := NewExporter(nagiosURL, conf.APIKey, *sslVerify, time.Duration(*nagiosAPITimeout)*time.Second)
	prometheus.MustRegister(exporter)

	log.Info("Using connection endpoint: ", *remoteAddress)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
			<head><title>Nagios Exporter</title></head>
			<body>
			<h1>Nagios Exporter</h1>
			<p><a href='` + *metricsPath + `'>Metrics</a></p>
			</body>
			</html>`))
		if err != nil {
			log.Fatal(err)
		}
	})

	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
