package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// 加密、校验与所有响应脱敏共用字段定义，避免 Go JSON 大小写兼容形成旁路。
var outboundSecretFields = map[string]string{
	"privatekey": "privateKey", "password": "password", "id": "id",
	"pass": "pass", "secretkey": "secretKey", "presharedkey": "preSharedKey",
}

// publicKey 仅在 REALITY 中是 password 的旧别名；WireGuard 对端公钥保持可见。
func isRealityPublicKey(parent, key string) bool {
	return strings.EqualFold(parent, "realitySettings") && strings.EqualFold(key, "publicKey")
}

func IsSensitiveConfigField(parent, key string) bool {
	return IsSensitiveConfigKey(key) || isRealityPublicKey(parent, key)
}

func IsSensitiveConfigKey(key string) bool {
	_, secret := outboundSecretFields[strings.ToLower(key)]
	return secret || strings.EqualFold(key, "email") || strings.EqualFold(key, "user")
}

func isUnsafeOutboundField(key string) bool {
	switch strings.ToLower(key) {
	case "redirect", "proxysettings", "reverse":
		return true
	}
	return false
}

// 解析前检查重复键；map 解码会吞掉重复项，不能用于这项检查。
func validateOutboundJSONKeys(raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var readValue func() error
	readValue = func() error {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		delim, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				token, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok {
					return fmt.Errorf("JSON 对象字段名无效")
				}
				lower := strings.ToLower(key)
				if seen[lower] {
					return fmt.Errorf("自定义出站包含重复字段 %q（不区分大小写）", key)
				}
				seen[lower] = true
				if isUnsafeOutboundField(key) {
					return fmt.Errorf("自定义出站不允许字段 %s", key)
				}
				canonical, known := outboundSecretFields[lower]
				if !known {
					switch lower {
					case "servers", "users", "vnext", "peers", "address", "port", "user", "email", "settings", "sockopt", "reserved":
						canonical, known = lower, true
					case "streamsettings":
						canonical, known = "streamSettings", true
					case "realitysettings":
						canonical, known = "realitySettings", true
					case "publickey":
						canonical, known = "publicKey", true
					case "dialerproxy":
						canonical, known = "dialerProxy", true
					case "targetstrategy":
						canonical, known = "targetStrategy", true
					case "domainstrategy":
						canonical, known = "domainStrategy", true
					}
				}
				if known && key != canonical {
					return fmt.Errorf("自定义出站字段 %q 请使用标准命名 %q", key, canonical)
				}
				if err := readValue(); err != nil {
					return err
				}
			}
		case '[':
			for dec.More() {
				if err := readValue(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("JSON 结构无效")
		}
		_, err = dec.Token()
		return err
	}
	if err := readValue(); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("只允许一个 JSON 值")
	}
	return nil
}

// 递归处理敏感字段，也覆盖 Xray 简写 settings 与高级传输参数中的凭据。
func transformSecretJSON(raw json.RawMessage, path string, fn secretTransformFunc) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	changed := false
	var walk func(any, string) error
	walk = func(value any, path string) error {
		switch obj := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(obj))
			for key := range obj {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parent := path[strings.LastIndexByte(path, '.')+1:]
			for _, key := range keys {
				fieldPath := path + "." + key
				if _, secret := outboundSecretFields[strings.ToLower(key)]; secret || isRealityPublicKey(parent, key) {
					if obj[key] != nil {
						if _, ok := obj[key].(string); !ok {
							return fmt.Errorf("%s 敏感凭据必须是字符串", fieldPath)
						}
					}
					if text, ok := obj[key].(string); ok && text != "" {
						next, err := fn(fieldPath, text)
						if err != nil {
							return err
						}
						if next != text {
							obj[key], changed = next, true
						}
						continue
					}
				}
				if err := walk(obj[key], fieldPath); err != nil {
					return err
				}
			}
		case []any:
			for i, item := range obj {
				if err := walk(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(value, path); err != nil {
		return nil, err
	}
	if !changed {
		return raw, nil
	}
	return marshalNoEscape(value)
}
