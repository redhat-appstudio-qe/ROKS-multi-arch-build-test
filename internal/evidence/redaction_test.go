package evidence

import (
	"strings"
	"testing"
)

func TestRedactNestedSensitiveFields(t *testing.T) {
	value := map[string]any{
		"name":   "run-1",
		"token":  "top-secret",
		"nested": map[string]any{"password": "also-secret", "value": "kept"},
		"items":  []any{map[string]any{"authorizationHeader": "bearer"}},
	}
	data, err := RedactedJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "top-secret") || strings.Contains(text, "also-secret") || strings.Contains(text, "bearer") {
		t.Fatalf("secret leaked: %s", text)
	}
	if !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, "kept") {
		t.Fatalf("unexpected redaction: %s", text)
	}
}
