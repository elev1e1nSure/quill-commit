package secretscan

import "testing"

func TestScanOpenAI(t *testing.T) {
	if !Scan([]byte("sk-or-v1-abc123def456ghi789jkl012mno345pqr678stu")) {
		t.Error("expected OpenAI key to match")
	}
	if Scan([]byte("sk-or-v1-")) { // too short
		t.Error("expected short prefix to not match")
	}
}

func TestScanAWS(t *testing.T) {
	if !Scan([]byte("AKIAIOSFODNN7EXAMPLE")) {
		t.Error("expected AWS key to match")
	}
	if Scan([]byte("AKIA123")) { // too short
		t.Error("expected short AWS key to not match")
	}
}

func TestScanGitHub(t *testing.T) {
	if !Scan([]byte("ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")) {
		t.Error("expected GitHub personal token to match")
	}
	if !Scan([]byte("ghs_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")) {
		t.Error("expected GitHub server token to match")
	}
	if Scan([]byte("ghp_xxx")) { // too short
		t.Error("expected short GitHub token to not match")
	}
}

func TestScanSlack(t *testing.T) {
	cases := []string{
		"xoxb-1234567890-1234567890-abcdef",
		"xoxp-1234567890-1234567890-abcdef",
		"xoxo-1234567890-1234567890-abcdef",
		"xoxa-1234567890-1234567890-abcdef",
		"xoxs-1234567890-1234567890-abcdef",
	}
	for _, c := range cases {
		if !Scan([]byte(c)) {
			t.Errorf("expected Slack token %q to match", c)
		}
	}
	if Scan([]byte("xoxb-123")) { // too short
		t.Error("expected short Slack token to not match")
	}
}

func TestScanGoogle(t *testing.T) {
	if !Scan([]byte("AIzaSyAabcdefghijklmnopqrstuvwxyz1234_5")) {
		t.Error("expected Google API key to match")
	}
	if Scan([]byte("AIza123")) { // too short
		t.Error("expected short Google key to not match")
	}
}

func TestScanNoFalsePositives(t *testing.T) {
	cases := []string{
		"some regular text without any tokens",
		`{"key": "value", "nested": {"foo": "bar"}}`,
		"base64 encoded data: d2hhdGV2ZXI=",
		"// TODO: fix this function",
		"sk-test-not-a-real-key",
		"AKIA-example-not-real",
	}
	for _, c := range cases {
		if Scan([]byte(c)) {
			t.Errorf("expected %q to not match", c)
		}
	}
}

func TestScanMultipleInOneLine(t *testing.T) {
	data := []byte("here is an OpenAI key sk-or-v1-abc123def456ghi789jkl012mno345pqr678stu and some normal text")
	if !Scan(data) {
		t.Error("expected line with embedded key to match")
	}
}
