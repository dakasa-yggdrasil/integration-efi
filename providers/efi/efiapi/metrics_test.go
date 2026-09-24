package efiapi

import "testing"

func TestClassifyPath_AutomaticWebhooksHaveTheirOwnBoundedLabel(t *testing.T) {
	cases := map[string]string{
		"/v2/webhookrec":             "automatic_webhook",
		"/v2/webhookcobr":            "automatic_webhook",
		"/v2/webhook/pix@dakasa.me":  "webhook",
		"/v3/gn/webhook/pix@x.test":  "webhook",
		"/v2/webhookrec?inicio=x":    "other",
		"/v2/webhookrecx":            "other",
		"/v2/webhook?inicio=a&fim=b": "other",
	}
	for path, want := range cases {
		if got := classifyPath(path); got != want {
			t.Errorf("classifyPath(%q) = %q, want %q", path, got, want)
		}
	}
}
