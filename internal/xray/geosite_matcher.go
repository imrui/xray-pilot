package xray

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

type geositeDomainType uint64

const (
	geositeDomainPlain      geositeDomainType = 0
	geositeDomainRegex      geositeDomainType = 1
	geositeDomainRootDomain geositeDomainType = 2
	geositeDomainFull       geositeDomainType = 3
)

type geositeDomainRule struct {
	Type  geositeDomainType
	Value string
	Regex *regexp.Regexp
	Attrs []string
}

// GeositeDB 提供轻量只读的 Xray geosite.dat 域名分类匹配器（基于 protowire，不污染全局 proto 注册表）。
type GeositeDB struct {
	sha256Hex  string
	rawEntries map[string][][]byte
	mu         sync.RWMutex
	parsed     map[string][]geositeDomainRule
	corrupt    map[string]bool
}

type cachedGeositeFile struct {
	modTimeNano int64
	size        int64
	db          *GeositeDB
}

var (
	geositeCacheMu sync.Mutex
	geositeCache   = map[string]cachedGeositeFile{}
)

// SHA256 返回当前 GeositeDB 原始数据的十六进制 SHA-256 摘要，用于与目标节点资产摘要核对一致性。
func (db *GeositeDB) SHA256() string {
	if db == nil {
		return ""
	}
	return db.sha256Hex
}

// ParseGeositeData 从 geosite.dat 二进制内容解析分类索引（按需延迟解码具体分类下的域名条目）。
func ParseGeositeData(data []byte) (*GeositeDB, error) {
	sum := sha256.Sum256(data)
	rawEntries := make(map[string][][]byte)
	b := data
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]
		if num == 1 && typ == protowire.BytesType {
			entryBytes, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			b = b[m:]
			code, domains, err := parseGeositeEntryHeader(entryBytes)
			if err != nil {
				return nil, err
			}
			if code != "" {
				upper := strings.ToUpper(code)
				rawEntries[upper] = append(rawEntries[upper], domains...)
			}
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			return nil, protowire.ParseError(m)
		}
		b = b[m:]
	}
	return &GeositeDB{
		sha256Hex:  hex.EncodeToString(sum[:]),
		rawEntries: rawEntries,
		parsed:     make(map[string][]geositeDomainRule),
		corrupt:    make(map[string]bool),
	}, nil
}

func parseGeositeEntryHeader(b []byte) (string, [][]byte, error) {
	var code string
	var domains [][]byte
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", nil, protowire.ParseError(n)
		}
		b = b[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			s, m := protowire.ConsumeString(b)
			if m < 0 {
				return "", nil, protowire.ParseError(m)
			}
			if !utf8.ValidString(s) {
				return "", nil, fmt.Errorf("geosite country_code 包含非法 UTF-8")
			}
			code = s
			b = b[m:]
		case num == 2 && typ == protowire.BytesType:
			rawDom, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return "", nil, protowire.ParseError(m)
			}
			domains = append(domains, rawDom)
			b = b[m:]
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return "", nil, protowire.ParseError(m)
			}
			b = b[m:]
		}
	}
	return code, domains, nil
}

func decodeGeositeDomainRule(b []byte) (geositeDomainRule, error) {
	var r geositeDomainRule
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return r, protowire.ParseError(n)
		}
		b = b[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return r, protowire.ParseError(m)
			}
			r.Type = geositeDomainType(v)
			b = b[m:]
		case num == 2 && typ == protowire.BytesType:
			s, m := protowire.ConsumeString(b)
			if m < 0 {
				return r, protowire.ParseError(m)
			}
			if !utf8.ValidString(s) {
				return r, fmt.Errorf("geosite domain value 包含非法 UTF-8")
			}
			r.Value = s
			b = b[m:]
		case num == 3 && typ == protowire.BytesType:
			attrBytes, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return r, protowire.ParseError(m)
			}
			key, err := decodeGeositeAttrKey(attrBytes)
			if err != nil {
				return r, err
			}
			// 保留原始属性键大小写，遵循 Xray 官方仅规范化过滤器并与原始键精确比较的语义。
			r.Attrs = append(r.Attrs, key)
			b = b[m:]
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return r, protowire.ParseError(m)
			}
			b = b[m:]
		}
	}
	if r.Type > geositeDomainFull {
		return r, fmt.Errorf("未知 geosite 域名匹配类型: %d", r.Type)
	}
	if r.Type == geositeDomainRegex {
		re, err := regexp.Compile(r.Value)
		if err != nil {
			return r, fmt.Errorf("无效 geosite 正则规则 %q: %w", r.Value, err)
		}
		r.Regex = re
	}
	return r, nil
}

// decodeGeositeAttrKey 完整消费 Attribute 消息字节，遵守 protobuf 标量字段末次值生效，并拒绝尾部畸形编码。
func decodeGeositeAttrKey(b []byte) (string, error) {
	var key string
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", protowire.ParseError(n)
		}
		b = b[n:]
		if num == 1 && typ == protowire.BytesType {
			s, m := protowire.ConsumeString(b)
			if m < 0 {
				return "", protowire.ParseError(m)
			}
			if !utf8.ValidString(s) {
				return "", fmt.Errorf("geosite attribute key 包含非法 UTF-8")
			}
			key = s
			b = b[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			return "", protowire.ParseError(m)
		}
		b = b[m:]
	}
	return key, nil
}

func (db *GeositeDB) rulesForCategory(upperCode string) ([]geositeDomainRule, bool) {
	if db == nil {
		return nil, false
	}
	db.mu.RLock()
	if db.corrupt[upperCode] {
		db.mu.RUnlock()
		return nil, false
	}
	if rules, ok := db.parsed[upperCode]; ok {
		db.mu.RUnlock()
		return rules, true
	}
	rawList, exists := db.rawEntries[upperCode]
	db.mu.RUnlock()
	if !exists {
		return nil, false
	}

	decoded := make([]geositeDomainRule, 0, len(rawList))
	for _, raw := range rawList {
		r, err := decodeGeositeDomainRule(raw)
		if err != nil {
			// 畸形 domain 或非法正则不得冒充“完整加载且明确不匹配”。
			db.mu.Lock()
			db.corrupt[upperCode] = true
			db.mu.Unlock()
			return nil, false
		}
		decoded = append(decoded, r)
	}
	db.mu.Lock()
	db.parsed[upperCode] = decoded
	db.mu.Unlock()
	return decoded, true
}

// Match 判断给定域名是否命中 geosite 规则（如 "category-ads-all" 或 "google@cn"）。
// 返回值 known=false 表示未提供已核验 geodata、分类不存在、语法含非法属性或数据损坏。
func (db *GeositeDB) Match(spec, host string) (matched bool, known bool) {
	if db == nil {
		return false, false
	}
	trimmedSpec := strings.TrimSpace(spec)
	if trimmedSpec == "" {
		return false, false
	}
	parts := strings.Split(trimmedSpec, "@")
	if parts[0] == "" {
		return false, false
	}
	filters := make([]string, 0, len(parts)-1)
	for _, rawAttr := range parts[1:] {
		attr := strings.ToLower(strings.TrimSpace(rawAttr))
		if attr == "" || strings.Contains(attr, "!") {
			return false, false
		}
		filters = append(filters, attr)
	}
	upperCode := strings.ToUpper(parts[0])
	rules, exists := db.rulesForCategory(upperCode)
	if !exists {
		return false, false
	}
	lowerHost := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	for _, rule := range rules {
		if !geositeAttrsMatch(rule.Attrs, filters) {
			continue
		}
		val := strings.ToLower(rule.Value)
		switch rule.Type {
		case geositeDomainPlain:
			if strings.Contains(lowerHost, val) {
				return true, true
			}
		case geositeDomainRegex:
			if rule.Regex != nil && rule.Regex.MatchString(lowerHost) {
				return true, true
			}
		case geositeDomainRootDomain:
			if lowerHost == val || strings.HasSuffix(lowerHost, "."+val) {
				return true, true
			}
		case geositeDomainFull:
			if lowerHost == val {
				return true, true
			}
		}
	}
	return false, true
}

// geositeAttrsMatch 对齐 Xray 26.3.27 / 26.11.9 官方属性过滤语义：过滤器已转小写，与原始键精确比较并取存在性 AND。
func geositeAttrsMatch(ruleAttrs, filters []string) bool {
	for _, f := range filters {
		has := false
		for _, a := range ruleAttrs {
			if a == f {
				has = true
				break
			}
		}
		if !has {
			return false
		}
	}
	return true
}

func loadGeositeFileCached(path string) *GeositeDB {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	geositeCacheMu.Lock()
	if hit, ok := geositeCache[path]; ok && hit.modTimeNano == info.ModTime().UnixNano() && hit.size == info.Size() {
		geositeCacheMu.Unlock()
		return hit.db
	}
	geositeCacheMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	db, err := ParseGeositeData(data)
	if err != nil {
		return nil
	}
	geositeCacheMu.Lock()
	geositeCache[path] = cachedGeositeFile{
		modTimeNano: info.ModTime().UnixNano(),
		size:        info.Size(),
		db:          db,
	}
	geositeCacheMu.Unlock()
	return db
}

// ResolveDiagnosticLocalGeositeDB 仅用于生成非阻断诊断提示，绝不作为未知远端节点的硬拒绝依据。
func ResolveDiagnosticLocalGeositeDB(explicitPath string) *GeositeDB {
	if explicitPath != "" {
		return loadGeositeFileCached(explicitPath)
	}
	var candidates []string
	if assetDir := strings.TrimSpace(os.Getenv("XRAY_LOCATION_ASSET")); assetDir != "" {
		candidates = append(candidates, filepath.Join(assetDir, "geosite.dat"))
	}
	if testBin := strings.TrimSpace(os.Getenv("XRAY_TEST_BINARY")); testBin != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(testBin), "geosite.dat"))
	}
	candidates = append(candidates,
		"/usr/local/share/xray/geosite.dat",
		"/usr/share/xray/geosite.dat",
	)
	for _, p := range candidates {
		if db := loadGeositeFileCached(p); db != nil {
			return db
		}
	}
	return nil
}
