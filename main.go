package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// https://stackoverflow.com/a/16491396
type Config struct {
	APIKey string
}

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
