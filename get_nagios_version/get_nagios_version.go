package get_nagios_version

import (
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

func GetLatestNagiosXIVersion(NagiosXIURL string) (version string, err error) {

	// Fetch the HTML source data from the URL
	resp, err := http.Get(NagiosXIURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse the HTML data into a tree structure
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", err
	}

	// https://pkg.go.dev/golang.org/x/net/html#Parse
	// recursive function seems to be best practice
	var traverse func(*html.Node) string
	traverse = func(node *html.Node) string {
		if node.Type == html.TextNode {
			if strings.HasPrefix(node.Data, "xi-") {
				// the first `xi-` string is the latest NagiosXI version in semver
				return node.Data
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			result := traverse(child)
			if result != "" {
				return result
			}
		}
		return ""
	}

	// traverse the HTML parse tree and return the version if found
	version = traverse(doc)

	return version, nil
}
