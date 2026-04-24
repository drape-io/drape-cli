package api

// getInt extracts an integer pointer from a map, handling JSON's float64 encoding.
func getInt(m map[string]any, key string) *int {
	if v, ok := m[key].(float64); ok {
		n := int(v)
		return &n
	}
	return nil
}

// getIntVal extracts an integer value from a map, defaulting to 0.
func getIntVal(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

// getString extracts a string pointer from a map.
func getString(m map[string]any, key string) *string {
	if v, ok := m[key].(string); ok {
		return &v
	}
	return nil
}

// getStringVal extracts a string value from a map, defaulting to "".
func getStringVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getBool extracts a bool from a map, defaulting to false.
func getBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// getBoolPtr extracts a *bool from a map. Returns nil when the key is absent
// or not a boolean — distinguishing "not present" from "present and false".
func getBoolPtr(m map[string]any, key string) *bool {
	if v, ok := m[key].(bool); ok {
		return &v
	}
	return nil
}

// getStringSlice extracts a []string from a []any in a map.
func getStringSlice(m map[string]any, key string) []string {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
