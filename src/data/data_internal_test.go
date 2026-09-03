package data

// Internal tests for the data package — cover the JSON error/empty fallback
// path in Languages() which cannot be reached from the external _test package
// because languagesJSON is unexported.

import "testing"

// TestLanguages_InvalidJSON_Fallback confirms that corrupt JSON returns the
// default ["text"] slice without panicking.
func TestLanguages_InvalidJSON_Fallback(t *testing.T) {
	orig := languagesJSON
	languagesJSON = []byte("not-valid-json")
	defer func() { languagesJSON = orig }()

	langs := Languages()
	if len(langs) != 1 || langs[0] != "text" {
		t.Errorf("corrupt JSON: expected [\"text\"], got %v", langs)
	}
}

// TestLanguages_EmptyList_Fallback confirms that valid JSON with an empty
// languages array returns the default ["text"] slice.
func TestLanguages_EmptyList_Fallback(t *testing.T) {
	orig := languagesJSON
	languagesJSON = []byte(`{"languages":[]}`)
	defer func() { languagesJSON = orig }()

	langs := Languages()
	if len(langs) != 1 || langs[0] != "text" {
		t.Errorf("empty list: expected [\"text\"], got %v", langs)
	}
}
