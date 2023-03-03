package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/wbollock/nagiosxi_exporter/get_nagios_version"
)

func TestGetStringFromWebpage(t *testing.T) {
	// Create a test server that will return a specific string when called
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<!DOCTYPE html>
		<html>
		<head>
				<title>Nagios XI &middot; Previous Versions</title><link rel="stylesheet" href="https://assets.nagios.com/pageparts/styles/A.bootstrap.3.min.css.pagespeed.cf.G6_26bvhk8.css" type="text/css"/>
		</head>
		<body>
		<div class="container">
			<div class="row">
				<div class="col-sm-12">
					<h1>Nagios XI - Previous Versions</h1>
					<table class="table table-striped">
						<thead>
							<tr>
								<th style="width:50px">Major</th>
								<th style="width:220px">Version</th>
								<th>Size</th>
								<th>Modified</th>
								<th>Checksum (sha256sum)</th>
							</tr>
						</thead>
						<tbody>
		<tr>  <td>5</td>
						<td><a href='5/xi-5.9.3.tar.gz' onclick="ga('send', 'event', 'nagiosxi', 'Download', 'source');">xi-5.9.3</a></td>
						<td>76.55M</td>
						<td>02/1/23 06:53</td>
						<td>3f0170080064f215b16ca89d07e9ab9ec91d93936a921fae2051c5cf56f18baa</td></tr><tr>  <td>5</td>
						<td><a href='5/xi-5.9.2.tar.gz' onclick="ga('send', 'event', 'nagiosxi', 'Download', 'source');">xi-5.9.2</a></td>
						<td>77.42M</td>
						<td>12/5/22 08:20</td>"`))
		if err != nil {
			// handle the error here
			log.Fatal(err)
		}
	}))
	defer testServer.Close()

	// Call the function with the URL of the test server
	result, err := get_nagios_version.GetLatestNagiosXIVersion(testServer.URL)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check that the result matches the expected string
	expected := "xi-5.9.3"
	if result != expected {
		t.Errorf("Expected %q, but got %q", expected, result)
	}
}
