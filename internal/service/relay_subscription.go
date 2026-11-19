package service

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"gorm.io/gorm"
)

const KeyRelayEnabled = "subscription.relay_enabled"
const KeyRelayPrefix = "subscription.relay_prefix"

var ErrRelayID = errors.New("内网中转订阅 ID 只能使用字母、数字、点、短横线或下划线，且不能为 . 或 ..")
var ErrRelayDuplicate = errors.New("此内网中转订阅 ID 已分配给其他用户")
var ErrRelaySettings = errors.New("请填写有效的 HTTP(S) 内网中转订阅地址前缀，不能包含凭据、查询参数或片段")
var relayIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func relayID(raw string) (*string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return nil, nil
	}
	if !relayIDPattern.MatchString(id) || id == "." || id == ".." {
		return nil, ErrRelayID
	}
	return &id, nil
}

func relayPrefix(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(raw, "?#\\") {
		return "", ErrRelaySettings
	}
	if port := u.Port(); port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return "", ErrRelaySettings
		}
	}
	return strings.TrimRight(u.String(), "/") + "/", nil
}

// 同一快照读取开关和前缀，避免关停或更换前缀期间拼出不一致的链接。
func relaySettings(db *gorm.DB) (map[string]string, error) {
	var rows []entity.SystemSetting
	if err := db.Where("key IN ?", []string{KeyRelayEnabled, KeyRelayPrefix}).Find(&rows).Error; err != nil {
		return nil, err
	}
	values := map[string]string{KeyRelayEnabled: "false", KeyRelayPrefix: ""}
	for _, s := range rows {
		values[s.Key] = s.Value
	}
	return values, nil
}

func validateRelaySettings(tx *gorm.DB, patch map[string]string) error {
	_, enabled := patch[KeyRelayEnabled]
	_, prefix := patch[KeyRelayPrefix]
	if !enabled && !prefix {
		return nil
	}
	values, err := relaySettings(tx)
	if err != nil {
		return err
	}
	for key := range values {
		if v, ok := patch[key]; ok {
			values[key] = strings.TrimSpace(v)
		}
	}
	if values[KeyRelayEnabled] != "false" && values[KeyRelayEnabled] != "true" {
		return ErrRelaySettings
	}
	if values[KeyRelayPrefix] != "" || values[KeyRelayEnabled] == "true" {
		value, err := relayPrefix(values[KeyRelayPrefix])
		if err != nil {
			return err
		}
		values[KeyRelayPrefix] = value
	}
	if prefix {
		patch[KeyRelayPrefix] = values[KeyRelayPrefix]
	}
	if enabled {
		patch[KeyRelayEnabled] = values[KeyRelayEnabled]
	}
	return nil
}

func currentRelayPrefix() string {
	values, err := relaySettings(repository.DB)
	if err != nil || values[KeyRelayEnabled] != "true" {
		return ""
	}
	base, _ := relayPrefix(values[KeyRelayPrefix])
	return base
}

func relayURL(base string, u *entity.User) string {
	if base == "" || u.RelaySubscriptionID == nil {
		return ""
	}
	id, err := relayID(*u.RelaySubscriptionID)
	if err != nil || id == nil {
		return ""
	}
	return base + *id
}

func distributedRelayURL(u *entity.User) string {
	if u.RelaySubscriptionID == nil || !u.EffectiveActive(time.Now()) {
		return ""
	}
	return relayURL(currentRelayPrefix(), u)
}

func relayUserError(err error) error {
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique") && strings.Contains(message, "relay_subscription_id") {
			return ErrRelayDuplicate
		}
	}
	return err
}
