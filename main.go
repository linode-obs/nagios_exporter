package main

import (
	"fmt"
	"os/exec"
	"log"
	"net/http"


	"github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
// api notes
// http://139.144.16.126/nagiosxi/help/?xiwindow=api-object-reference.php

// starter API calls and metrics
// remember get more ideas with `/usr/local/nagios/bin/nagiostats -c /usr/local/nagios/etc/nagios.cfg`

// curl
// curl -XGET "http://139.144.16.126/nagiosxi/api/v1/objects/hosts?apikey=<key>"


// Hosts
// Total Hosts  - objects/hostatus | jq '.recordcount'
// Hosts Actively Checked:  - objects/hoststatus | jq '.recordcount'
// note - check_type 1 = passive
// Host Passively Checked:                 0
// "hoststatus": [
// { ..
// "check_type" : 0 (active), 1 (passive)
// }
// Hosts Up/Down/Unreach
// "current_state": "0", (up) 1 (down) 2 (unreach)
// Hosts Flapping
//  "is_flapping": "0",
// Hosts in Downtime
// "scheduled_downtime_depth": "1", (1 = downtime, 0 = no downtime.. the depth is a little concerning)

// Services
// Total Services:                         25
// recordcount

// Services Checked:                       24
// "has_been_checked": "0", (1 not checked yet )
// Services Scheduled:                     25
// "should be scheduled": "0", (1 not scheculed )
// Services Actively Checked:              25
// "check_type": "0",
// Services Passively Checked:             0
// "check_type": "1", - just weird that the passive check I made is 0 for this..
// Services Ok/Warn/Unk/Crit:              24 / 0 / 1 / 0
// Services Flapping:                      0
// "is_flapping": "0", (1 is flapping)
// Services In Downtime:                   0
// // "scheduled_downtime_depth": "1", (1 = downtime, 0 = no downtime)

// Checks
// Active host checks scheduled
// "check_type": "0", and  "should_be_scheduled": "1",
// Passive host checks scheduled
// "check_type": "1", and  "should_be_scheduled": "1",

// Misc
// Version info: api/v1/system/info

// begin by initializing prometheus metrics

// spin up a prometheus client

// gather metrics and push them to the client

// parse a config - let the user specify a port and where the application lives, maybe also a debug mode and where to log to
// 	also path to nagios config and nagiosxi
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9101", nil))


}

