package xray

import "encoding/json"

// SameNonUserConfig prevents a user hot update from clearing pending routing/key changes.
func SameNonUserConfig(a, b string) bool {
	strip := func(raw string) (string, error) {
		var v map[string]any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return "", err
		}
		inbounds, ok := v["inbounds"].([]any)
		if !ok {
			return "", json.Unmarshal([]byte("!"), &v)
		}
		for _, in := range inbounds {
			inbound, ok := in.(map[string]any)
			if !ok {
				continue
			}
			if settings, ok := inbound["settings"].(map[string]any); ok {
				delete(settings, "clients")
			}
		}
		data, err := json.Marshal(v)
		return string(data), err
	}
	x, e1 := strip(a)
	y, e2 := strip(b)
	return e1 == nil && e2 == nil && x == y
}
