package config

import "testing"

func TestValidateFooterHTML(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantErr   bool
		wantEmpty bool
	}{
		{"empty passes through", "", false, true},
		{"space sentinel passes through", " ", false, false},
		{"safe html passes through unchanged", "<p>Powered by <strong>MyCompany</strong></p>", false, false},
		{"script only is fully stripped and rejected", "<script>alert('xss')</script>", true, false},
		{"event handler only is fully stripped and rejected", `<img src="x" onerror="alert('xss')">`, true, false},
		{"iframe only is fully stripped and rejected", `<iframe src="https://evil.com"></iframe>`, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateFooterHTML(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateFooterHTML(%q): expected error, got nil (sanitized=%q)", tc.input, got)
				}
				if got != "" {
					t.Fatalf("ValidateFooterHTML(%q): expected empty result on error, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateFooterHTML(%q): unexpected error: %v", tc.input, err)
			}
			if tc.wantEmpty && got != "" {
				t.Fatalf("ValidateFooterHTML(%q): expected empty result, got %q", tc.input, got)
			}
		})
	}
}

func TestValidateFooterHTMLPartiallyStrippedNotRejected(t *testing.T) {
	// Content with a mix of safe and unsafe elements sanitizes to non-empty
	// output and must not be rejected — only fully-stripped content errors.
	got, err := ValidateFooterHTML(`<p>Built with</p><script>alert(1)</script>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty sanitized output")
	}
}
