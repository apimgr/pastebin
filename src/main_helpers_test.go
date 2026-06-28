package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestNormalizeArgs verifies short-flag expansion and single-dash long-flag
// conversion performed before flag parsing.
func TestNormalizeArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "short help and version expand",
			in:   []string{"-h", "-v"},
			want: []string{"--help", "--version"},
		},
		{
			name: "single dash long flag passes through unchanged (spec: only -h/-v are short flags)",
			in:   []string{"-config", "/etc"},
			want: []string{"-config", "/etc"},
		},
		{
			name: "double dash flag is untouched",
			in:   []string{"--port", "8080"},
			want: []string{"--port", "8080"},
		},
		{
			name: "short single-char unknown flag is untouched",
			in:   []string{"-x"},
			want: []string{"-x"},
		},
		{
			name: "positional argument is untouched",
			in:   []string{"scheduler", "list"},
			want: []string{"scheduler", "list"},
		},
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("normalizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("normalizeArgs(%v)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestApplyColor verifies the --color flag mutates NO_COLOR per spec PART 8.
func TestApplyColor(t *testing.T) {
	t.Run("no sets NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		os.Unsetenv("NO_COLOR")
		applyColor("no")
		if os.Getenv("NO_COLOR") != "1" {
			t.Fatalf("applyColor(no): NO_COLOR = %q, want 1", os.Getenv("NO_COLOR"))
		}
	})

	t.Run("never alias sets NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		os.Unsetenv("NO_COLOR")
		applyColor("never")
		if os.Getenv("NO_COLOR") != "1" {
			t.Fatalf("applyColor(never): NO_COLOR = %q, want 1", os.Getenv("NO_COLOR"))
		}
	})

	t.Run("yes unsets NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		applyColor("yes")
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			t.Fatalf("applyColor(yes): NO_COLOR should be unset")
		}
	})

	t.Run("always alias unsets NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		applyColor("always")
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			t.Fatalf("applyColor(always): NO_COLOR should be unset")
		}
	})

	t.Run("auto leaves NO_COLOR set", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		applyColor("auto")
		if os.Getenv("NO_COLOR") != "1" {
			t.Fatalf("applyColor(auto): NO_COLOR = %q, want unchanged 1", os.Getenv("NO_COLOR"))
		}
	})

	t.Run("empty leaves NO_COLOR unset", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		os.Unsetenv("NO_COLOR")
		applyColor("")
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			t.Fatalf("applyColor(empty): NO_COLOR should remain unset")
		}
	})
}

// TestPrintHelp verifies the help output contains the binary name and the key
// flag sections documented in the spec.
func TestPrintHelp(t *testing.T) {
	var buf strings.Builder
	printHelp(&buf, "pastebin")
	got := buf.String()

	wantSubstrings := []string{
		"pastebin",
		"Usage:",
		"--help",
		"--version",
		"--port",
		"--daemon",
		"--color",
		"scheduler list",
		"--clean-expired",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(got, s) {
			t.Errorf("printHelp output missing %q", s)
		}
	}
}

// TestLogSchedErr verifies the helper tolerates a nil error and logs a non-nil
// error without panicking.
func TestLogSchedErr(t *testing.T) {
	logSchedErr(nil)
	logSchedErr(context.DeadlineExceeded)
}

// TestNewHTTPRequest verifies the User-Agent header is set and bad methods error.
func TestNewHTTPRequest(t *testing.T) {
	t.Run("sets user agent", func(t *testing.T) {
		req, err := newHTTPRequest(context.Background(), http.MethodGet, "http://example.com/", nil)
		if err != nil {
			t.Fatalf("newHTTPRequest: %v", err)
		}
		ua := req.Header.Get("User-Agent")
		if !strings.HasPrefix(ua, "pastebin/") {
			t.Fatalf("User-Agent = %q, want pastebin/ prefix", ua)
		}
	})

	t.Run("invalid method errors", func(t *testing.T) {
		_, err := newHTTPRequest(context.Background(), "BAD METHOD", "http://example.com/", nil)
		if err == nil {
			t.Fatal("newHTTPRequest with invalid method: want error, got nil")
		}
	})
}

// TestDoHTTP verifies the helper executes a request against a live test server.
func TestDoHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "pastebin/") {
			t.Errorf("server saw User-Agent %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := newHTTPRequest(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("newHTTPRequest: %v", err)
	}
	resp, err := doHTTP(req)
	if err != nil {
		t.Fatalf("doHTTP: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
