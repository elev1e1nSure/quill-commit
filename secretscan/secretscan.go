package secretscan

import "regexp"

var knownPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-or-v1-[a-zA-Z0-9]{20,}`),          // OpenAI
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                    // AWS Access Key ID
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),               // GitHub personal access token
	regexp.MustCompile(`ghs_[a-zA-Z0-9]{36}`),               // GitHub server-to-server token
	regexp.MustCompile(`xox[bpoas]-[a-zA-Z0-9-]{10,}`),      // Slack tokens
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),              // Google API key
}

// Scan reports whether data contains any known secret signature.
func Scan(data []byte) bool {
	for _, re := range knownPatterns {
		if re.Match(data) {
			return true
		}
	}
	return false
}
