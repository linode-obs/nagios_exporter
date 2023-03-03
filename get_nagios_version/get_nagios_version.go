package get_nagios_version

import (
	"fmt"
	"io/ioutil" // TODO: ioutil outdated
	"net/http"
	"strings"

	"golang.org/x/net/html"
	log "github.com/sirupsen/logrus"
)

func GetLatestNagiosXIVersion(NagiosXIURL string) (version string, err error) {

	// Fetch the HTML source data from the URL
	resp, err := http.Get(NagiosXIURL)
	if err != nil {
		fmt.Println("Error fetching HTML:", err)
		return
	}
	defer resp.Body.Close()

	// Read the HTML data
	htmlData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		return "", err
	}

	// Convert HTML data to a tokenizer
	r := strings.NewReader(string(htmlData))
	tokenizer := html.NewTokenizer(r)

	// Iterate through all tokens
	for {
		tokenType := tokenizer.Next()

		if tokenType == html.ErrorToken {
			break
		} else if tokenType == html.TextToken {
			text := tokenizer.Token().Data
			if strings.HasPrefix(text, "xi-") {
				// the first `xi-` string is the latest NagiosXI version in semver
				return text, nil
			}
		}

	}
	return "", err
}