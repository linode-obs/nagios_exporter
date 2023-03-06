package get_nagios_version

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/html"
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
	htmlData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Info(err)
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
