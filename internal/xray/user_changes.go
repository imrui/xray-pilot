package xray

import (
	"encoding/json"
	"fmt"
	"sort"
)

type UserChange struct {
	Tag, Email, UUID, Protocol string
	Remove                     bool
}

// DiffUsers uses the current complete snapshot, never a possibly stale API request.
func DiffUsers(old, next string) ([]UserChange, error) {
	parse := func(raw string) (map[string]UserChange, error) {
		var cfg struct {
			Inbounds []struct {
				Tag, Protocol  string
				StreamSettings struct{ Security string }
				Settings       struct {
					Clients []struct{ ID, Password, Email string }
				}
			}
		}
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, err
		}
		out := map[string]UserChange{}
		for _, in := range cfg.Inbounds {
			if in.Protocol != "vless" && in.Protocol != "trojan" {
				continue
			}
			for _, u := range in.Settings.Clients {
				if u.Email == "" {
					return nil, fmt.Errorf("缺少用户统计标识")
				}
				id := u.ID
				proto := "vless-reality"
				if in.Protocol == "vless" && in.StreamSettings.Security != "reality" {
					proto = "vless-ws-tls"
				}
				if in.Protocol == "trojan" {
					id = u.Password
					proto = "trojan"
				}
				out[in.Tag+"\x00"+u.Email] = UserChange{Tag: in.Tag, Email: u.Email, UUID: id, Protocol: proto}
			}
		}
		return out, nil
	}
	a, err := parse(old)
	if err != nil {
		return nil, err
	}
	b, err := parse(next)
	if err != nil {
		return nil, err
	}
	var removals, adds []UserChange
	for key, u := range a {
		if v, ok := b[key]; !ok || v.UUID != u.UUID {
			u.Remove = true
			removals = append(removals, u)
		}
	}
	for key, u := range b {
		if v, ok := a[key]; !ok || v.UUID != u.UUID {
			adds = append(adds, u)
		}
	}
	less := func(x []UserChange) {
		sort.Slice(x, func(i, j int) bool { return x[i].Tag+x[i].Email < x[j].Tag+x[j].Email })
	}
	less(removals)
	less(adds)
	return append(removals, adds...), nil
}
