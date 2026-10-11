package evidence

import (
	"encoding/json"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitiveFragments = []string{"token", "password", "secret", "authorization", "credential"}

// IsSensitiveKey reports whether key contains a credential-related fragment.
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, fragment := range sensitiveFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// Redact recursively replaces values under sensitive map keys with a redaction marker.
func Redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if IsSensitiveKey(key) {
				result[key] = redactedValue
			} else {
				result[key] = Redact(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = Redact(item)
		}
		return result
	default:
		return value
	}
}

// RedactedJSON marshals value as indented JSON after redacting sensitive fields.
func RedactedJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	return json.MarshalIndent(Redact(generic), "", "  ")
}
