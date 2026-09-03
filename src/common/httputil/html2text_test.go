package httputil

import (
	"strings"
	"testing"
)

func TestHTML2TextConverter_H1(t *testing.T) {
	out := HTML2TextConverter("<h1>Hello</h1>", 20)
	if !strings.Contains(out, "HELLO") {
		t.Errorf("h1 output missing uppercase title: %q", out)
	}
	if !strings.Contains(out, "═") {
		t.Errorf("h1 output missing box-drawing border: %q", out)
	}
}

func TestHTML2TextConverter_H2(t *testing.T) {
	out := HTML2TextConverter("<h2>Section</h2>", 80)
	if !strings.Contains(out, "─── Section ───") {
		t.Errorf("h2 output: got %q", out)
	}
}

func TestHTML2TextConverter_H3(t *testing.T) {
	out := HTML2TextConverter("<h3>Sub</h3>", 80)
	if !strings.Contains(out, "► Sub") {
		t.Errorf("h3 output: got %q", out)
	}
}

func TestHTML2TextConverter_Paragraph(t *testing.T) {
	out := HTML2TextConverter("<p>Hello world</p>", 80)
	if !strings.Contains(out, "Hello world") {
		t.Errorf("p output: got %q", out)
	}
}

func TestHTML2TextConverter_UnorderedList(t *testing.T) {
	out := HTML2TextConverter("<ul><li>Apple</li><li>Banana</li></ul>", 80)
	if !strings.Contains(out, "• Apple") {
		t.Errorf("ul output missing bullet: %q", out)
	}
	if !strings.Contains(out, "• Banana") {
		t.Errorf("ul output missing second bullet: %q", out)
	}
}

func TestHTML2TextConverter_OrderedList(t *testing.T) {
	out := HTML2TextConverter("<ol><li>First</li><li>Second</li></ol>", 80)
	if !strings.Contains(out, "1. First") {
		t.Errorf("ol output missing first item: %q", out)
	}
	if !strings.Contains(out, "2. Second") {
		t.Errorf("ol output missing second item: %q", out)
	}
}

func TestHTML2TextConverter_Link(t *testing.T) {
	out := HTML2TextConverter(`<a href="/foo">Click here</a>`, 80)
	if !strings.Contains(out, "Click here [/foo]") {
		t.Errorf("link output: got %q", out)
	}
}

func TestHTML2TextConverter_Bold(t *testing.T) {
	out := HTML2TextConverter("<strong>Important</strong>", 80)
	if !strings.Contains(out, "*Important*") {
		t.Errorf("strong output: got %q", out)
	}
}

func TestHTML2TextConverter_Code(t *testing.T) {
	out := HTML2TextConverter("<code>foo()</code>", 80)
	if !strings.Contains(out, "`foo()`") {
		t.Errorf("code output: got %q", out)
	}
}

func TestHTML2TextConverter_HR(t *testing.T) {
	out := HTML2TextConverter("<hr>", 20)
	if !strings.Contains(out, "────────────────────") {
		t.Errorf("hr output: got %q", out)
	}
}

func TestHTML2TextConverter_Blockquote(t *testing.T) {
	out := HTML2TextConverter("<blockquote>Quoted text</blockquote>", 80)
	if !strings.Contains(out, "│ Quoted text") {
		t.Errorf("blockquote output: got %q", out)
	}
}

func TestHTML2TextConverter_SkipsScript(t *testing.T) {
	out := HTML2TextConverter("<p>visible</p><script>alert('x')</script>", 80)
	if strings.Contains(out, "alert") {
		t.Errorf("script content leaked into output: %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("visible content missing: %q", out)
	}
}

func TestHTML2TextConverter_SkipsForm(t *testing.T) {
	out := HTML2TextConverter(`<form><input type="text"><button>Submit</button></form><p>after</p>`, 80)
	if strings.Contains(out, "Submit") {
		t.Errorf("form button leaked: %q", out)
	}
}

func TestHTML2TextConverter_Table(t *testing.T) {
	tableHTML := `<table><tr><th>Name</th><th>Value</th></tr><tr><td>foo</td><td>bar</td></tr></table>`
	out := HTML2TextConverter(tableHTML, 80)
	if !strings.Contains(out, "Name") || !strings.Contains(out, "foo") {
		t.Errorf("table output missing expected content: %q", out)
	}
	if !strings.Contains(out, "│") {
		t.Errorf("table output missing box-drawing chars: %q", out)
	}
}

func TestHTML2TextConverter_InvalidFallback(t *testing.T) {
	// Feeds something that cannot be valid HTML structure — parser still recovers,
	// but the fallback stripTags path should not panic.
	out := HTML2TextConverter("not html at all", 80)
	if out == "" {
		t.Error("expected non-empty output for plain text input")
	}
}

func TestWordWrap(t *testing.T) {
	cases := []struct {
		text  string
		width int
		lines int
	}{
		{"hello world", 20, 1},
		{"hello world", 5, 2},
		{"", 80, 0},
	}
	for _, tc := range cases {
		got := wordWrap(tc.text, tc.width)
		lines := strings.Split(got, "\n")
		if tc.lines == 0 {
			if got != "" {
				t.Errorf("wordWrap(%q, %d) = %q, want empty", tc.text, tc.width, got)
			}
			continue
		}
		if len(lines) != tc.lines {
			t.Errorf("wordWrap(%q, %d): got %d lines, want %d", tc.text, tc.width, len(lines), tc.lines)
		}
	}
}

func TestCenterText(t *testing.T) {
	out := centerText("HI", 10)
	if !strings.Contains(out, "HI") {
		t.Errorf("centerText missing content: %q", out)
	}
	if len(out) < len("HI") {
		t.Errorf("centerText too short: %q", out)
	}
}

func TestStripTags(t *testing.T) {
	out := stripTags("<b>bold</b> and <i>italic</i>")
	if strings.Contains(out, "<") {
		t.Errorf("stripTags left tags: %q", out)
	}
	if !strings.Contains(out, "bold") {
		t.Errorf("stripTags removed content: %q", out)
	}
}
