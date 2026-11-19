package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/crypto"
)

type DNSService struct {
	endpoint string
	client   *http.Client
}

func NewDNSService() *DNSService {
	return &DNSService{
		endpoint: "https://api.cloudflare.com/client/v4",
		client: &http.Client{
			Timeout:       8 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type dnsRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type DNSUpstreamError struct {
	message    string
	transient  bool
	retryAfter time.Duration
}

func (e *DNSUpstreamError) Error() string { return e.message }

func (s *DNSService) request(ctx context.Context, cfg entity.DNSConfig, method, path string, body []byte, result any) error {
	token, err := crypto.Decrypt(cfg.TokenCipher)
	if err != nil || token == "" {
		return errors.New("DNS API Token 不可用，请检查密钥配置")
	}
	req, err := http.NewRequestWithContext(ctx, method, s.endpoint+"/zones/"+cfg.ZoneID+"/dns_records"+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("DNS 请求地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		return &DNSUpstreamError{"连接 Cloudflare 失败或请求超时", true, 0}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		e := &DNSUpstreamError{message: fmt.Sprintf("Cloudflare 返回 HTTP %d", res.StatusCode), transient: res.StatusCode == 429 || res.StatusCode == 408 || res.StatusCode >= 500}
		if res.StatusCode == 401 || res.StatusCode == 403 {
			e.message += "，请检查 Token 与 Zone 权限"
		}
		if retry := res.Header.Get("Retry-After"); retry != "" {
			if seconds, err := strconv.ParseUint(retry, 10, 32); err == nil {
				e.retryAfter = time.Duration(seconds) * time.Second
			} else if at, err := http.ParseTime(retry); err == nil {
				e.retryAfter = time.Until(at)
			}
		}
		return e
	}
	var reply struct {
		Success bool
		Result  json.RawMessage
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&reply); err != nil || !reply.Success || json.Unmarshal(reply.Result, result) != nil {
		return &DNSUpstreamError{"Cloudflare 响应未能确认操作结果", true, 0}
	}
	return nil
}

const errRecordConflict DNSValidationError = "DNS 记录与绑定不一致，请核对后重新绑定"

func (s *DNSService) lookup(ctx context.Context, cfg entity.DNSConfig, name, kind string) (dnsRecord, error) {
	var rows []dnsRecord
	// 只取两条即可判定同名同类型是否唯一。
	query := url.Values{"name.exact": {name}, "type": {kind}, "per_page": {"2"}}
	if err := s.request(ctx, cfg, "GET", "?"+query.Encode(), nil, &rows); err != nil {
		return dnsRecord{}, err
	}
	if len(rows) == 0 {
		query.Set("type", "CNAME")
		if err := s.request(ctx, cfg, "GET", "?"+query.Encode(), nil, &rows); err != nil {
			return dnsRecord{}, err
		}
		if len(rows) > 0 {
			return dnsRecord{}, fmt.Errorf("%w：该域名使用 CNAME，请选择已有的 %s 记录", errRecordConflict, kind)
		}
	}
	if len(rows) != 1 {
		return dnsRecord{}, fmt.Errorf("%w：需要唯一的 %s 记录，查询到 %d 条", errRecordConflict, kind, len(rows))
	}
	row := rows[0]
	ip, actual := entity.DNSAddress(row.Content)
	if entity.DNSName(row.Name) != name || row.Type != kind || actual != kind || row.ID == "" {
		return dnsRecord{}, errRecordConflict
	}
	row.Content = ip
	return row, nil
}
