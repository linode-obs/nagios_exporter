package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
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
const systemuserAPI = "/system/user"

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
			MinExecutionTime float64 `json:"min_execution_time,string"`
			MinLatency       float64 `json:"min_latency,string"`
		} `json:"activehostcheckperf"`
		Activehostchecks struct {
			Val1  float64 `json:"val1,string"`
			Val15 float64 `json:"val15,string"`
			Val5  float64 `json:"val5,string"`
		} `json:"activehostchecks"`
		Activeservicecheckperf struct {
			AvgExecutionTime float64 `json:"avg_execution_time,string"`
			AvgLatency       float64 `json:"avg_latency,string"`
			MaxExecutionTime float64 `json:"max_execution_time,string"`
			MaxLatency       float64 `json:"max_latency,string"`
			MinExecutionTime float64 `json:"min_execution_time,string"`
			MinLatency       float64 `json:"min_latency,string"`
		} `json:"activeservicecheckperf"`
		Activeservicechecks struct {
			Val1  float64 `json:"val1,string"`
			Val15 float64 `json:"val15,string"`
			Val5  float64 `json:"val5,string"`
		} `json:"activeservicechecks"`
		Passivehostchecks struct {
			Val1  float64 `json:"val1,string"`
			Val15 float64 `json:"val15,string"`
			Val5  float64 `json:"val5,string"`
		} `json:"passivehostchecks"`
		Passiveservicechecks struct {
			Val1  float64 `json:"val1,string"`
			Val15 float64 `json:"val15,string"`
			Val5  float64 `json:"val5,string"`
		} `json:"passiveservicechecks"`
		Updated string `json:"updated"`
	} `json:"nagioscore"`
}

type systemInfo struct {
	Version string `json:"version"`
}

// generated with https://github.com/bashtian/jsonutils
type hostStatus struct {
	Recordcount float64 `json:"recordcount"`
	Hoststatus  []struct {
		HostObjectID               float64 `json:"host_object_id,string"`
		CheckType                  float64 `json:"check_type,string"`
		CurrentState               float64 `json:"current_state,string"`
		IsFlapping                 float64 `json:"is_flapping,string"`
		ScheduledDowntimeDepth     float64 `json:"scheduled_downtime_depth,string"`
		ProblemHasBeenAcknowledged float64 `json:"problem_has_been_acknowledged,string"`
	} `json:"hoststatus"`
}

type serviceStatus struct {
	Recordcount   float64 `json:"recordcount"`
	Servicestatus []struct {
		HasBeenChecked             float64 `json:"has_been_checked,string"`
		ShouldBeScheduled          float64 `json:"should_be_scheduled,string"`
		CheckType                  float64 `json:"check_type,string"`
		CurrentState               float64 `json:"current_state,string"`
		IsFlapping                 float64 `json:"is_flapping,string"`
		ScheduledDowntimeDepth     float64 `json:"scheduled_downtime_depth,string"`
		ProblemHasBeenAcknowledged float64 `json:"problem_has_been_acknowledged,string"`
	} `json:"servicestatus"`
}

type userStatus struct {
	// yes, this field is named records even though every other endpoint is `recordcount`...
	Recordcount float64 `json:"records"`
	Userstatus  []struct {
		Admin   float64 `json:"admin,string"`
		Enabled float64 `json:"enabled,string"`
	} `json:"users"`
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
	hostsTotal                = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_total"), "Amount of hosts present in configuration", nil, nil)
	hostsCheckedTotal         = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_checked_total"), "Amount of hosts checked", []string{"check_type"}, nil)
	hostsStatus               = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_status_total"), "Amount of hosts in different states", []string{"status"}, nil)
	hostsDowntime             = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "hosts_downtime_total"), "Amount of hosts in downtime", nil, nil)
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
	// operator is min/max/avg exposed by Nagios XI API
	// performance_type is latency/execution
	// technically there is no such thing as a check_type of passive for these metrics
	hostchecksPerformance    = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "host_checks_performance_seconds"), "Host checks performance", []string{"check_type", "performance_type", "operator"}, nil)
	servicechecksPerformance = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "service_checks_performance_seconds"), "Service checks performance", []string{"check_type", "performance_type", "operator"}, nil)

	// Users
	usersTotal      = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "users_total"), "Amount of users present on the system", nil, nil)
	usersPrivileges = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "users_privileges_total"), "Amount of admin or regular users", []string{"privileges"}, nil)
	usersStatus     = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "users_status_total"), "Amount of disabled or enabled users", []string{"status"}, nil)
)

type Exporter struct {
	nagiosEndpoint, nagiosAPIKey string
	sslVerify                    bool
	nagiosAPITimeout             time.Duration
	nagiostatsPath               string
	nagiosconfigPath             string
}

func NewExporter(nagiosEndpoint, nagiosAPIKey string, sslVerify bool, nagiosAPITimeout time.Duration, nagiostatsPath string, nagiosconfigPath string) *Exporter {
	return &Exporter{
		nagiosEndpoint:   nagiosEndpoint,
		nagiosAPIKey:     nagiosAPIKey,
		sslVerify:        sslVerify,
		nagiosAPITimeout: nagiosAPITimeout,
		nagiostatsPath:   nagiostatsPath,
		nagiosconfigPath: nagiosconfigPath,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// Nagios status
	ch <- up
	// Hosts
	ch <- hostsTotal
	ch <- hostsStatus
	ch <- hostsDowntime
	if e.nagiostatsPath == "" {
		// nagiostats has no support for ACK status
		ch <- hostsProblemsAcknowledged
		ch <- hostsCheckedTotal
	}
	// Services
	ch <- servicesTotal
	ch <- servicesStatus
	ch <- servicesDowntime
	if e.nagiostatsPath == "" {
		ch <- servicesProblemsAcknowledged
		ch <- servicesCheckedTotal
	}
	// System
	ch <- versionInfo
	ch <- buildInfo
	// System Detail
	ch <- hostchecks
	ch <- servicechecks
	ch <- hostchecksPerformance
	ch <- servicechecksPerformance
	// Users
	if e.nagiostatsPath == "" {
		// we cannot get user information from Nagios Core 3/4
		ch <- usersTotal
		ch <- usersPrivileges
		ch <- usersStatus
	}
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

func (e *Exporter) TestNagiosstatsBinary(nagiostatsPath string, nagiosconfigPath string) float64 {

	cmd := exec.Command(nagiostatsPath, "-c", nagiosconfigPath)
	err := cmd.Run()

	if err != nil {
		log.Fatal(err)
		return 0
	}

	return 1
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	if e.nagiostatsPath == "" {
		nagiosStatus := e.TestNagiosConnectivity(e.sslVerify, e.nagiosAPITimeout)

		if nagiosStatus == 0 {
			log.Warn("Cannot connect to Nagios endpoint")
		}

		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, nagiosStatus,
		)

		e.QueryAPIsAndUpdateMetrics(ch, e.sslVerify, e.nagiosAPITimeout)
	} else {
		nagiosStatus := e.TestNagiosstatsBinary(e.nagiostatsPath, e.nagiosconfigPath)
		if nagiosStatus == 0 {
			log.Warn("Cannot execute nagiostats: ", e.nagiostatsPath)
		}

		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, nagiosStatus,
		)

		e.QueryNagiostatsAndUpdateMetrics(ch, e.nagiostatsPath, e.nagiosconfigPath)
	}
}

// NagiosXI only supports submitting an API token as a URL parameter, so we need to scrub the API key from HTTP client errors
func sanitizeAPIKeyErrors(err error) error {
	var re = regexp.MustCompile("(apikey=)(.*)")
	sanitizedString := re.ReplaceAllString(err.Error(), "${1}<redactedAPIKey>")

	return errors.New(sanitizedString)
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
		log.Warn(sanitizeAPIKeyErrors(err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Prometheus")

	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(sanitizeAPIKeyErrors(err))
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	} else {
		log.Fatal("HTTP response body is nil - check API connectivity")
	}

	body, readErr := io.ReadAll(resp.Body)

	if readErr != nil {
		log.Fatal(sanitizeAPIKeyErrors(readErr))
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

	// host status
	hoststatusURL := e.nagiosEndpoint + hoststatusAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(hoststatusURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", systeminfoAPI)

	hostStatusObject := hostStatus{}

	jsonErr = json.Unmarshal(body, &hostStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	var hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount, hostsFlapCount, hostsDowntimeCount, hostsProblemsAcknowledgedCount float64

	// iterate through nested json
	for _, v := range hostStatusObject.Hoststatus {

		// for every host
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
		hostsProblemsAcknowledged, prometheus.GaugeValue, hostsProblemsAcknowledgedCount,
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

	var servicesCount, servicesScheduledCount, servicesActiveCheckCount,
		servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesCriticalCount,
		servicesUnknownCount, servicesFlapCount, servicesDowntimeCount, servicesProblemsAcknowledgedCount float64

	for _, v := range serviceStatusObject.Servicestatus {

		servicesCount++

		if v.ShouldBeScheduled == 0 {
			servicesScheduledCount++
		}

		if v.CheckType == 0 {
			servicesActiveCheckCount++
		} else {
			servicesPassiveCheckCount++
		}

		switch currentstate := v.CurrentState; currentstate {
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
		servicesProblemsAcknowledged, prometheus.GaugeValue, servicesProblemsAcknowledgedCount,
	)

	// system status
	systemStatusDetailURL := e.nagiosEndpoint + systemstatusDetailAPI + "?apikey=" + e.nagiosAPIKey

	body = QueryAPIs(systemStatusDetailURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", systemstatusDetailAPI)

	systemStatusDetailObject := systemStatusDetail{}

	jsonErr = json.Unmarshal(body, &systemStatusDetailObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	// user information
	// we also need to tack on the optional parameter of `advanced` to get privilege information
	systemUserURL := e.nagiosEndpoint + systemuserAPI + "?apikey=" + e.nagiosAPIKey + "&advanced=1"

	body = QueryAPIs(systemUserURL, sslVerify, nagiosAPITimeout)
	log.Debug("Queried API: ", systemuserAPI)

	userStatusObject := userStatus{}

	jsonErr = json.Unmarshal(body, &userStatusObject)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	var usersAdminCount, usersRegularCount, usersEnabledCount, usersDisabledCount float64

	ch <- prometheus.MustNewConstMetric(
		usersTotal, prometheus.GaugeValue, userStatusObject.Recordcount,
	)

	for _, v := range userStatusObject.Userstatus {

		if v.Admin == 1 {
			usersAdminCount++
		} else {
			usersRegularCount++
		}

		if v.Enabled == 1 {
			usersEnabledCount++
		} else {
			usersDisabledCount++
		}
	}

	ch <- prometheus.MustNewConstMetric(
		usersStatus, prometheus.GaugeValue, usersEnabledCount, "enabled",
	)

	ch <- prometheus.MustNewConstMetric(
		usersStatus, prometheus.GaugeValue, usersDisabledCount, "disabled",
	)

	ch <- prometheus.MustNewConstMetric(
		usersPrivileges, prometheus.GaugeValue, usersAdminCount, "admin",
	)

	ch <- prometheus.MustNewConstMetric(
		usersPrivileges, prometheus.GaugeValue, usersRegularCount, "user",
	)

	e.UpdateCommonMetrics(ch, hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount,
		hostsFlapCount, hostsDowntimeCount,
		servicesCount, servicesActiveCheckCount, servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesCriticalCount, servicesUnknownCount,
		servicesFlapCount, servicesDowntimeCount,
		systemStatusDetailObject.Nagioscore.Activehostchecks.Val1, systemStatusDetailObject.Nagioscore.Activehostchecks.Val5, systemStatusDetailObject.Nagioscore.Activehostchecks.Val15,
		systemStatusDetailObject.Nagioscore.Passivehostchecks.Val1, systemStatusDetailObject.Nagioscore.Passivehostchecks.Val5, systemStatusDetailObject.Nagioscore.Passivehostchecks.Val15,
		systemStatusDetailObject.Nagioscore.Activeservicechecks.Val1, systemStatusDetailObject.Nagioscore.Activeservicechecks.Val5, systemStatusDetailObject.Nagioscore.Activeservicechecks.Val15,
		systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val1, systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val5, systemStatusDetailObject.Nagioscore.Passiveservicechecks.Val15, systemStatusDetailObject.Nagioscore.Activehostcheckperf.AvgLatency, systemStatusDetailObject.Nagioscore.Activehostcheckperf.MinLatency, systemStatusDetailObject.Nagioscore.Activehostcheckperf.MaxLatency, systemStatusDetailObject.Nagioscore.Activehostcheckperf.AvgExecutionTime, systemStatusDetailObject.Nagioscore.Activehostcheckperf.MinExecutionTime, systemStatusDetailObject.Nagioscore.Activehostcheckperf.MaxExecutionTime, systemStatusDetailObject.Nagioscore.Activeservicecheckperf.AvgLatency, systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MinLatency, systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MaxLatency, systemStatusDetailObject.Nagioscore.Activeservicecheckperf.AvgExecutionTime, systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MinExecutionTime, systemStatusDetailObject.Nagioscore.Activeservicecheckperf.MaxExecutionTime)

	log.Info("Endpoint scraped and metrics updated")
}

func (e *Exporter) UpdateCommonMetrics(ch chan<- prometheus.Metric, hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount,
	hostsFlapCount, hostsDowntimeCount, servicesCount, servicesActiveCheckCount, servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesCriticalCount, servicesUnknownCount,
	servicesFlapCount, servicesDowntimeCount,
	activehostchecks1m, activehostchecks5m, activehostchecks15m, passivehostchecks1m, passivehostchecks5m, passivehostchecks15m,
	activeservicechecks1m, activeservicechecks5m, activeservicechecks15m, passiveservicechecks1m, passiveservicechecks5m, passiveservicechecks15m, activehostchecklatencyavg, activehostchecklatencymin, activehostchecklatencymax, activehostcheckexecutionavg, activehostcheckexecutionmin, activehostcheckexecutionmax, activeservicechecklatencyavg, activeservicechecklatencymin, activeservicechecklatencymax, activeservicecheckexecutionavg, activeservicecheckexecutionmin, activeservicecheckexecutionmax float64) {

	// Metrics common to both collection options

	ch <- prometheus.MustNewConstMetric(
		buildInfo, prometheus.GaugeValue, 1, Version, BuildDate, Commit,
	)

	// host status

	ch <- prometheus.MustNewConstMetric(
		hostsTotal, prometheus.GaugeValue, hostsCount,
	)

	ch <- prometheus.MustNewConstMetric(
		hostsCheckedTotal, prometheus.GaugeValue, hostsActiveCheckCount, "active",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsCheckedTotal, prometheus.GaugeValue, hostsPassiveCheckCount, "passive",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, hostsUpCount, "up",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, hostsDownCount, "down",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, hostsUnreachableCount, "unreachable",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsStatus, prometheus.GaugeValue, hostsFlapCount, "flapping",
	)

	ch <- prometheus.MustNewConstMetric(
		hostsDowntime, prometheus.GaugeValue, hostsDowntimeCount,
	)

	// service status

	ch <- prometheus.MustNewConstMetric(
		servicesTotal, prometheus.GaugeValue, servicesCount,
	)

	ch <- prometheus.MustNewConstMetric(
		servicesCheckedTotal, prometheus.GaugeValue, servicesActiveCheckCount, "active",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesCheckedTotal, prometheus.GaugeValue, servicesPassiveCheckCount, "passive",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, servicesOkCount, "ok",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, servicesWarnCount, "warn",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, servicesCriticalCount, "critical",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, servicesUnknownCount, "unknown",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesStatus, prometheus.GaugeValue, servicesFlapCount, "flapping",
	)

	ch <- prometheus.MustNewConstMetric(
		servicesDowntime, prometheus.GaugeValue, servicesDowntimeCount,
	)

	activeHostCheckSum := activehostchecks1m + activehostchecks5m + activehostchecks15m

	ch <- prometheus.MustNewConstHistogram(
		hostchecks, uint64(activeHostCheckSum), activeHostCheckSum, map[float64]uint64{
			1:  uint64(activehostchecks1m),
			5:  uint64(activehostchecks5m),
			15: uint64(activehostchecks15m)}, "active",
	)

	passiveHostCheckSum := passivehostchecks1m + passivehostchecks5m + passivehostchecks15m

	ch <- prometheus.MustNewConstHistogram(
		hostchecks, uint64(passiveHostCheckSum), passiveHostCheckSum, map[float64]uint64{
			1:  uint64(passivehostchecks1m),
			5:  uint64(passivehostchecks5m),
			15: uint64(passivehostchecks15m)}, "passive",
	)

	activeserviceCheckSum := activeservicechecks1m + activeservicechecks5m + activeservicechecks15m

	ch <- prometheus.MustNewConstHistogram(
		servicechecks, uint64(activeserviceCheckSum), activeserviceCheckSum, map[float64]uint64{
			1:  uint64(activeservicechecks1m),
			5:  uint64(activeservicechecks5m),
			15: uint64(activeservicechecks15m)}, "active",
	)

	passiveserviceCheckSum := passiveservicechecks1m + passiveservicechecks5m + passiveservicechecks15m

	ch <- prometheus.MustNewConstHistogram(
		servicechecks, uint64(passiveserviceCheckSum), passiveserviceCheckSum, map[float64]uint64{
			1:  uint64(passiveservicechecks1m),
			5:  uint64(passiveservicechecks5m),
			15: uint64(passiveservicechecks15m)}, "passive",
	)

	// active host check performance
	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, activehostchecklatencyavg, "active", "latency", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, activehostchecklatencymin, "active", "latency", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, activehostchecklatencymax, "active", "latency", "max",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, activehostcheckexecutionavg, "active", "execution", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, activehostcheckexecutionmin, "active", "execution", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		hostchecksPerformance, prometheus.GaugeValue, activehostcheckexecutionmax, "active", "execution", "max",
	)

	// active service check performance
	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, activeservicechecklatencyavg, "active", "latency", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, activeservicechecklatencymin, "active", "latency", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, activeservicechecklatencymax, "active", "latency", "max",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, activeservicecheckexecutionavg, "active", "execution", "avg",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, activeservicecheckexecutionmin, "active", "execution", "min",
	)

	ch <- prometheus.MustNewConstMetric(
		servicechecksPerformance, prometheus.GaugeValue, activeservicecheckexecutionmax, "active", "execution", "max",
	)

}

func (e *Exporter) QueryNagiostatsAndUpdateMetrics(ch chan<- prometheus.Metric, nagiostatsPath string, nagiosconfigPath string) {
	// to get specific values, we output them in MRTG format
	// we pass a comma seperated string of MRTG data - must be manually kept up to date
	mrtgList := "NAGIOSVERSION,NUMHOSTS,NUMHSTACTCHK60M,NUMHSTPSVCHK60M,NUMHSTUP,NUMHSTDOWN,NUMHSTUNR,NUMHSTFLAPPING,NUMHSTDOWNTIME,NUMSERVICES,NUMSVCACTCHK60M,NUMSVCPSVCHK60M,NUMSVCOK,NUMSVCWARN,NUMSVCUNKN,NUMSVCCRIT,NUMSVCFLAPPING,NUMSVCDOWNTIME,NUMHSTACTCHK1M,NUMHSTACTCHK5M,NUMHSTACTCHK15M,NUMHSTPSVCHK1M,NUMHSTPSVCHK5M,NUMHSTPSVCHK15M,NUMSVCACTCHK1M,NUMSVCACTCHK5M,NUMSVCACTCHK15M,NUMSVCPSVCHK1M,NUMSVCPSVCHK5M,NUMSVCPSVCHK15M,AVGACTHSTLAT,MINACTHSTLAT,MAXACTHSTLAT,AVGACTHSTEXT,MINACTHSTEXT,MAXACTHSTEXT,AVGACTSVCLAT,MINACTSVCLAT,MAXACTSVCLAT,AVGACTSVCEXT,MINACTSVCEXT,MAXACTSVCEXT"

	// -m = mrtg; -D = use comma as delimiter, -d = MRTG list input
	cmd := exec.Command(nagiostatsPath, "-c", nagiosconfigPath, "-m", "-D", ",", "-d", mrtgList)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()

	if err != nil {
		log.Fatal(err)
	}
	log.Debug("Queried nagiostats: ", out.String())
	// input our comma seperated list as metrics
	cmdSplice := strings.Split(out.String(), ",")

	// Need float64 values for metrics
	metricSlice := make([]float64, 0, len(cmdSplice))

	for _, metric := range cmdSplice {
		metric, _ := strconv.ParseFloat(metric, 64)
		metricSlice = append(metricSlice, metric)
	}

	var nagiosVersion string = cmdSplice[0] // NAGIOSVERSION
	ch <- prometheus.MustNewConstMetric(
		// we do want this value to be a string though as it's a label
		versionInfo, prometheus.GaugeValue, 1, nagiosVersion,
	)

	// host status
	var hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount, hostsFlapCount, hostsDowntimeCount float64

	// maintaining variables for each of these makes it slightly easier to parse
	// its really horrible but not sure there's a better way

	hostsCount = metricSlice[1]             // NUMHOSTS
	hostsActiveCheckCount = metricSlice[2]  // NUMHSTACTCHK60M - technically only hosts actively checked in last hour
	hostsPassiveCheckCount = metricSlice[3] // NUMHSTPSVCHK60M
	hostsUpCount = metricSlice[4]           // NUMHSTUP
	hostsDownCount = metricSlice[5]         // NUMHSTDOWN
	hostsUnreachableCount = metricSlice[6]  // NUMHSTUNR
	hostsFlapCount = metricSlice[7]         // NUMHSTFLAPPING
	hostsDowntimeCount = metricSlice[8]     // NUMHSTDOWNTIME

	// service status
	var servicesCount, servicesActiveCheckCount,
		servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesUnknownCount, servicesCriticalCount, servicesFlapCount, servicesDowntimeCount float64

	servicesCount = metricSlice[9]              // NUMSERVICES
	servicesActiveCheckCount = metricSlice[10]  // NUMSVCACTCHK60M
	servicesPassiveCheckCount = metricSlice[11] // NUMSVCPSVCHK60M
	servicesOkCount = metricSlice[12]           // NUMSVCOK
	servicesWarnCount = metricSlice[13]         // NUMSVCWARN
	servicesUnknownCount = metricSlice[14]      // NUMSVCUNKN
	servicesCriticalCount = metricSlice[15]     // NUMSVCCRIT
	servicesFlapCount = metricSlice[16]         // NUMSVCFLAPPING
	servicesDowntimeCount = metricSlice[17]     // NUMSVCDOWNTIME

	// check performance
	var activehostchecks1m, activehostchecks5m, activehostchecks15m,
		passivehostchecks1m, passivehostchecks5m, passivehostchecks15m,
		activeservicechecks1m, activeservicechecks5m, activeservicechecks15m,
		passiveservicechecks1m, passiveservicechecks5m, passiveservicechecks15m float64

	activehostchecks1m = metricSlice[18]   // NUMHSTACTCHK1M
	activehostchecks5m = metricSlice[19]   // NUMHSTACTCHK5M
	activehostchecks15m = metricSlice[20]  // NUMHSTACTCHK15M
	passivehostchecks1m = metricSlice[21]  // NUMHSTPSVCHK1M
	passivehostchecks5m = metricSlice[22]  // NUMHSTPSVCHK5M
	passivehostchecks15m = metricSlice[23] // NUMHSTPSVCHK15M

	activeservicechecks1m = metricSlice[24]   // NUMSVCACTCHK1M
	activeservicechecks5m = metricSlice[25]   // NUMSVCACTCHK5M
	activeservicechecks15m = metricSlice[26]  // NUMSVCACTCHK15M
	passiveservicechecks1m = metricSlice[27]  // NUMSVCPSVCHK1M
	passiveservicechecks5m = metricSlice[28]  // NUMSVCPSVCHK5M
	passiveservicechecks15m = metricSlice[29] // NUMSVCPSVCHK15M

	var activehostchecklatencyavg, activehostchecklatencymin, activehostchecklatencymax,
		activehostcheckexecutionavg, activehostcheckexecutionmin, activehostcheckexecutionmax,
		activeservicechecklatencyavg, activeservicechecklatencymin, activeservicechecklatencymax,
		activeservicecheckexecutionavg, activeservicecheckexecutionmin, activeservicecheckexecutionmax float64

	activehostchecklatencyavg = metricSlice[30] // AVGACTHSTLAT
	activehostchecklatencymin = metricSlice[31] // MINACTHSTLAT
	activehostchecklatencymax = metricSlice[32] // MAXACTHSTLAT

	activehostcheckexecutionavg = metricSlice[33] // AVGACTHSTEXT
	activehostcheckexecutionmin = metricSlice[34] // MINACTHSTEXT
	activehostcheckexecutionmax = metricSlice[35] // MAXACTHSTEXT

	activeservicechecklatencyavg = metricSlice[36] // AVGACTSVCLAT
	activeservicechecklatencymin = metricSlice[37] // MINACTSVCLAT
	activeservicechecklatencymax = metricSlice[38] // MAXACTSVCLAT

	activeservicecheckexecutionavg = metricSlice[39] // AVGACTSVCEXT
	activeservicecheckexecutionmin = metricSlice[40] // MINACTSVCEXT
	activeservicecheckexecutionmax = metricSlice[41] // MAXACTSVCEXT

	e.UpdateCommonMetrics(ch, hostsCount, hostsActiveCheckCount, hostsPassiveCheckCount, hostsUpCount, hostsDownCount, hostsUnreachableCount,
		hostsFlapCount, hostsDowntimeCount,
		servicesCount, servicesActiveCheckCount, servicesPassiveCheckCount, servicesOkCount, servicesWarnCount, servicesCriticalCount, servicesUnknownCount,
		servicesFlapCount, servicesDowntimeCount,
		activehostchecks1m, activehostchecks5m, activehostchecks15m,
		passivehostchecks1m, passivehostchecks5m, passivehostchecks15m,
		activeservicechecks1m, activeservicechecks5m, activeservicechecks15m,
		passiveservicechecks1m, passiveservicechecks5m, passiveservicechecks15m, activehostchecklatencyavg, activehostchecklatencymin, activehostchecklatencymax, activehostcheckexecutionavg, activehostcheckexecutionmin, activehostcheckexecutionmax, activeservicechecklatencyavg, activeservicechecklatencymin, activeservicechecklatencymax, activeservicecheckexecutionavg, activeservicecheckexecutionmin, activeservicecheckexecutionmax)

	log.Info("Nagiostats scraped and metrics updated")
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
		statsBinary = flag.String("nagios.stats_binary", "",
			"Path of nagiostats binary and configuration (e.g /usr/local/nagios/bin/nagiostats -c /usr/local/nagios/etc/nagios.cfg)")
		nagiosConfigPath = flag.String("nagios.config_path", "",
			"Nagios configuration path for use with nagiostats binary (e.g /usr/local/nagios/etc/nagios.cfg)")
	)

	flag.Parse()

	if *logLevel == "debug" {
		log.SetLevel(log.DebugLevel)
		log.Debug("Log level set to debug")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	var nagiosURL string
	var conf Config

	// if we _aren't_ using nagiostats, it'll be a blank string
	if *statsBinary == "" {
		conf = ReadConfig(*configPath)

		formatter := nagiosFormatter{}
		formatter.APIKey = conf.APIKey
		log.SetFormatter(&formatter)

		nagiosURL = *remoteAddress + nagiosAPIVersion + apiSlug
	} else {
		// if we're using nagiostats, set a dummy API key here
		conf.APIKey = ""
	}

	// convert timeout flag to seconds
	exporter := NewExporter(nagiosURL, conf.APIKey, *sslVerify, time.Duration(*nagiosAPITimeout)*time.Second, *statsBinary, *nagiosConfigPath)
	prometheus.MustRegister(exporter)

	if *statsBinary == "" {
		log.Info("Using connection endpoint: ", *remoteAddress)
	} else {
		log.Info("Using nagiostats binary: ", *statsBinary)
		log.Info("Using Nagios configiration: ", *nagiosConfigPath)
	}

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
