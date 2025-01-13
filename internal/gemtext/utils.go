package gemtext

import "strings"

type parsedLink struct {
	url   string
	label string
}

func parseLinkLine(line string) parsedLink {
	line = strings.TrimSpace(line[2:])
	var (
		urlStr = line
		label  = urlStr
	)

	if i := strings.IndexAny(urlStr, " \t"); i > -1 {
		urlStr, label = urlStr[:i], strings.TrimSpace(urlStr[i:])
	}

	return parsedLink{url: urlStr, label: label}
}
