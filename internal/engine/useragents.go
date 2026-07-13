package engine

import "strings"

var userAgentProfiles = map[string]string{
	"chrome":     DefaultUserAgent,
	"safari":     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
	"firefox":    "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.0; rv:125.0) Gecko/20100101 Firefox/125.0",
	"googlebot":  "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
	"bingbot":    "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
	"seaportal":  "seaportal/0.x (+https://github.com/pinchtab/seaportal)",
	"search-bot": "Mozilla/5.0 (compatible; SearchBot/1.0)",
}

func ResolveUserAgent(s string) string {
	if s == "" {
		return DefaultUserAgent
	}
	if ua, ok := userAgentProfiles[strings.ToLower(s)]; ok {
		return ua
	}
	return s
}
