package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type balancerStrategyAlias BalancerStrategy
type balancerAlias Balancer
type observatoryAlias Observatory
type dnsConfigAlias DNSConfig

var balancerStrategyKnownKeys = map[string]string{"type": "type"}
var balancerKnownKeys = map[string]string{
	"tag": "tag", "remark": "remark", "selector": "selector", "strategy": "strategy", "fallbacktag": "fallbackTag",
}
var observatoryKnownKeys = map[string]string{
	"subjectselector": "subjectSelector", "probeurl": "probeURL", "probeinterval": "probeInterval", "enableconcurrency": "enableConcurrency",
}
var dnsConfigKnownKeys = map[string]string{
	"enabled": "enabled", "querystrategy": "queryStrategy", "disablecache": "disableCache", "disablefallback": "disableFallback",
	"disablefallbackifmatch": "disableFallbackIfMatch", "hosts": "hosts", "servers": "servers",
}

// 扩展字段只属于所在配置实体，不能注入面板托管骨架或改写已知字段。
func validateRoutingExtensions(extra map[string]json.RawMessage, kind string) error {
	var known map[string]string
	switch kind {
	case "dns":
		known = dnsConfigKnownKeys
	case "DNS 服务器":
		known = dnsServerKnownKeys
	case "balancer":
		known = balancerKnownKeys
	case "balancer.strategy":
		known = balancerStrategyKnownKeys
	case "observatory":
		known = observatoryKnownKeys
	}
	for k, v := range extra {
		if _, exists := known[strings.ToLower(k)]; exists {
			return fmt.Errorf("%s 扩展不能覆盖已知字段 %q", kind, k)
		}
		switch strings.ToLower(k) {
		case "api", "inbounds", "outbounds", "routing", "stats", "policy", "burstobservatory":
			return fmt.Errorf("%s 扩展不能包含系统字段 %q", kind, k)
		case "tag":
			if kind == "dns" || kind == "DNS 服务器" {
				var tag string
				if k != "tag" || json.Unmarshal(v, &tag) != nil {
					return fmt.Errorf("%s.tag 必须是标准命名的字符串字段", kind)
				}
				if tag != "" {
					if err := validateOutboundTagFormat(tag); err != nil {
						return fmt.Errorf("%s.tag: %w", kind, err)
					}
				}
			}
		case "settings":
			if kind == "balancer.strategy" {
				if k != "settings" {
					return fmt.Errorf("balancer.strategy 请使用标准命名 settings")
				}
				if _, err := decodeJSONObject(v); err != nil {
					return fmt.Errorf("balancer.strategy.settings: %w", err)
				}
			}
		}
	}
	return nil
}

func decodeRoutingExtensions(data []byte, target any, known map[string]string, kind string) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("%s 必须是 JSON 对象", kind)
	}
	seen := make(map[string]bool)
	var extra map[string]json.RawMessage
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("%s 字段名称无效", kind)
		}
		lower := strings.ToLower(key)
		if seen[lower] {
			return nil, fmt.Errorf("%s 存在重复字段 %q", kind, key)
		}
		seen[lower] = true
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		if canonical, ok := known[lower]; ok {
			if key != canonical {
				return nil, fmt.Errorf("%s 字段 %q 请使用标准命名 %q", kind, key, canonical)
			}
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("%s 只允许一个 JSON 对象", kind)
	}
	if err := validateRoutingExtensions(extra, kind); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return nil, err
	}
	return extra, nil
}

func marshalRoutingExtensions(value any, extra map[string]json.RawMessage, known map[string]string) ([]byte, error) {
	base, err := marshalNoEscape(value)
	if err != nil || len(extra) == 0 {
		return base, err
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range extra {
		if _, isKnown := known[strings.ToLower(k)]; !isKnown {
			merged[k] = v
		}
	}
	return marshalNoEscape(merged)
}

func (s *BalancerStrategy) UnmarshalJSON(data []byte) error {
	var value balancerStrategyAlias
	extra, err := decodeRoutingExtensions(data, &value, balancerStrategyKnownKeys, "balancer.strategy")
	if err == nil {
		*s = BalancerStrategy(value)
		s.Extra = extra
	}
	return err
}

func (s BalancerStrategy) MarshalJSON() ([]byte, error) {
	return marshalRoutingExtensions(balancerStrategyAlias(s), s.Extra, balancerStrategyKnownKeys)
}

func (b *Balancer) UnmarshalJSON(data []byte) error {
	var value balancerAlias
	extra, err := decodeRoutingExtensions(data, &value, balancerKnownKeys, "balancer")
	if err == nil {
		*b = Balancer(value)
		b.Extra = extra
	}
	return err
}

func (b Balancer) MarshalJSON() ([]byte, error) {
	return marshalRoutingExtensions(balancerAlias(b), b.Extra, balancerKnownKeys)
}

func (o *Observatory) UnmarshalJSON(data []byte) error {
	var value observatoryAlias
	extra, err := decodeRoutingExtensions(data, &value, observatoryKnownKeys, "observatory")
	if err == nil {
		*o = Observatory(value)
		o.Extra = extra
	}
	return err
}

func (o Observatory) MarshalJSON() ([]byte, error) {
	return marshalRoutingExtensions(observatoryAlias(o), o.Extra, observatoryKnownKeys)
}

func (d *DNSConfig) UnmarshalJSON(data []byte) error {
	var value dnsConfigAlias
	extra, err := decodeRoutingExtensions(data, &value, dnsConfigKnownKeys, "dns")
	if err == nil {
		*d = DNSConfig(value)
		d.Extra = extra
	}
	return err
}

func (d DNSConfig) MarshalJSON() ([]byte, error) {
	return marshalRoutingExtensions(dnsConfigAlias(d), d.Extra, dnsConfigKnownKeys)
}
