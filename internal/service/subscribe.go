package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"encoding/json"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"github.com/imrui/xray-pilot/pkg/types"
)

// SubscribeService 订阅生成服务（支持多协议）
type SubscribeService struct {
	userRepo    *repository.UserRepository
	nodeRepo    *repository.NodeRepository
	profileRepo *repository.InboundProfileRepository
	trafficRepo *repository.TrafficRepository
	settingSvc  *SettingService
}

func NewSubscribeService() *SubscribeService {
	return &SubscribeService{
		userRepo:    repository.NewUserRepository(),
		nodeRepo:    repository.NewNodeRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
		trafficRepo: repository.NewTrafficRepository(),
		settingSvc:  NewSettingService(),
	}
}

func (s *SubscribeService) GetSetting(key string) string {
	return s.settingSvc.Get(key)
}

// GenerateSubscription 生成用户订阅内容（base64 编码，多协议 URI 列表）
func (s *SubscribeService) GenerateSubscription(token string) (string, error) {
	user, err := s.userRepo.FindByToken(token)
	if err != nil {
		return "", errors.New("无效的订阅令牌")
	}
	if !user.Active {
		return "", errors.New("用户已禁用")
	}
	// 检查有效期
	if user.ExpiresAt != nil && user.ExpiresAt.Before(time.Now()) {
		return "", errors.New("订阅已过期")
	}
	groupIDs := userGroupIDs(user.Groups)
	if len(groupIDs) == 0 {
		return base64.StdEncoding.EncodeToString([]byte("")), nil
	}

	// 优先返回健康节点，无健康节点时降级返回全部激活节点
	nodes, err := s.nodeRepo.FindHealthyByGroupIDs(groupIDs, true)
	if err != nil {
		return "", fmt.Errorf("查询节点失败: %w", err)
	}
	if len(nodes) == 0 {
		nodes, err = s.nodeRepo.FindHealthyByGroupIDs(groupIDs, false)
		if err != nil {
			return "", fmt.Errorf("查询节点失败: %w", err)
		}
	}
	sortNodesByName(nodes)

	var links []string
	for _, node := range nodes {
		node := node
		// 获取节点关联的协议密钥
		profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
		if err != nil {
			continue
		}
		for _, key := range profileKeys {
			key := key
			link := s.buildURI(user, &node, key.Profile, &key)
			if link != "" {
				links = append(links, link)
			}
		}
	}

	content := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(content)), nil
}

// buildURI 根据协议类型分发 URI 构建
func (s *SubscribeService) buildURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	if profile == nil {
		return ""
	}
	switch profile.Protocol {
	case types.ProtocolVlessReality:
		return s.buildVlessRealityURI(user, node, profile, key)
	case types.ProtocolVlessWSTLS:
		return s.buildVlessWSTLSURI(user, node, profile, key)
	case types.ProtocolTrojan:
		return s.buildTrojanURI(user, node, profile, key)
	case types.ProtocolHysteria2:
		return s.buildHysteria2URI(user, node, profile, key)
	}
	return ""
}

func (s *SubscribeService) buildVlessRealityURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	var ps types.VlessRealitySettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}
	var km types.RealityKeyMaterial
	if key.Settings != "" {
		_ = json.Unmarshal([]byte(key.Settings), &km)
	}

	publicKey := km.PublicKey
	// 解密私钥只为服务端，公钥可明文存储
	sni := ps.SNI
	if sni == "" {
		sni = "www.microsoft.com"
	}
	fp := ps.Fingerprint
	if fp == "" {
		fp = "chrome"
	}

	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("security", "reality")
	params.Set("type", "tcp")
	params.Set("flow", "xtls-rprx-vision")
	params.Set("fp", fp)
	params.Set("sni", sni)
	if publicKey != "" {
		params.Set("pbk", publicKey)
	}
	if len(km.ShortIds) > 0 && km.ShortIds[0] != "" {
		params.Set("sid", km.ShortIds[0])
	}

	remark := s.buildRemark(node, user, "vless", "reality")
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.PathEscape(remark))
}

func (s *SubscribeService) buildVlessWSTLSURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	var ps types.VlessWSTLSSettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}

	host := ps.Host
	if host == "" {
		host = node.ConnectAddr()
	}
	path := ps.Path
	if path == "" {
		path = "/"
	}

	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("security", "tls")
	params.Set("type", "ws")
	params.Set("host", host)
	params.Set("path", path)
	params.Set("sni", host)

	remark := s.buildRemark(node, user, "vless", "ws")
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.PathEscape(remark))
}

func (s *SubscribeService) buildTrojanURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	var ps types.TrojanSettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}

	sni := ps.SNI
	if sni == "" {
		sni = node.ConnectAddr()
	}

	params := url.Values{}
	params.Set("security", "tls")
	params.Set("sni", sni)
	params.Set("type", "tcp")

	remark := s.buildRemark(node, user, "trojan", "tcp")
	return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.PathEscape(remark))
}

func (s *SubscribeService) buildHysteria2URI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	var ps types.Hysteria2Settings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}

	sni := ps.SNI
	if sni == "" {
		sni = node.ConnectAddr()
	}

	params := url.Values{}
	params.Set("sni", sni)
	if ps.UpMbps > 0 {
		params.Set("upmbps", fmt.Sprintf("%d", ps.UpMbps))
	}
	if ps.DownMbps > 0 {
		params.Set("downmbps", fmt.Sprintf("%d", ps.DownMbps))
	}

	remark := s.buildRemark(node, user, "hy2", "udp")
	return fmt.Sprintf("hy2://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.PathEscape(remark))
}

// buildRemark 生成节点备注，支持格式占位符：
// {node_name} {username} {protocol} {transport} {region} {name}
func (s *SubscribeService) buildRemark(node *entity.Node, user *entity.User, protocol, transport string) string {
	format := s.settingSvc.Get(KeySubscriptionRemarkFormat)
	if format == "" {
		format = "{node_name}"
	}
	remark := format
	remark = strings.ReplaceAll(remark, "{node_name}", node.Name)
	remark = strings.ReplaceAll(remark, "{username}", user.Username)
	remark = strings.ReplaceAll(remark, "{protocol}", protocol)
	remark = strings.ReplaceAll(remark, "{transport}", transport)
	remark = strings.ReplaceAll(remark, "{region}", node.Region)
	remark = strings.ReplaceAll(remark, "{name}", node.Name) // 向后兼容
	remark = strings.Trim(remark, "- ")
	if remark == "" {
		remark = node.Name
	}
	return remark
}

// SubscribePageData 订阅信息页所需数据
type SubscribePageData struct {
	Username  string
	Active    bool
	ExpiresAt *time.Time
	Nodes     []NodeLinkData
	SubURL    string    // 主订阅链接：按 UA 智能识别格式
	SubLinks  []SubLink // 备用订阅链接：按客户端类型显式区分
	AltSubURL string    // 向后兼容：等价于 V2Ray Base64 链接（?format=v2ray）

	// 累计流量（来自 UserTrafficTotal，由 TrafficPoller 周期更新）
	// 当用户从未产生流量时，三项均为零值，模板侧据此决定是否提示"尚无流量记录"
	TrafficUpBytes       int64
	TrafficDownBytes     int64
	TrafficTotalBytes    int64  // 上下行之和，预计算避免模板侧加法
	TrafficUpHuman       string // 人类可读，预格式化避免模板侧逻辑
	TrafficDownHuman     string
	TrafficTotalHuman    string
	TrafficLastUpdatedAt *time.Time
}

// SubLink 分客户端订阅链接（用于备用订阅卡片）
type SubLink struct {
	Label  string // 卡片显示的客户端类别名
	Hint   string // 客户端示例，作为副标题
	Format string // 对应 ?format= 参数值
	URL    string // 完整 URL
}

// NodeLinkData 节点链接信息
type NodeLinkData struct {
	Name   string
	Region string
	Link   string
}

// GetUser 通过 token 获取用户（供 handler 读取 ExpiresAt 用于响应头）
func (s *SubscribeService) GetUser(token string) (*entity.User, error) {
	return s.userRepo.FindByToken(token)
}

// GetSubscribePageData 返回信息页所需的完整数据
func (s *SubscribeService) GetSubscribePageData(token string) (*SubscribePageData, error) {
	return s.GetSubscribePageDataWithBaseURL(token, "")
}

// GetSubscribePageDataWithBaseURL 返回信息页所需的完整数据
// baseURL 优先使用调用方传入值；为空时回退到系统配置。
func (s *SubscribeService) GetSubscribePageDataWithBaseURL(token, baseURL string) (*SubscribePageData, error) {
	user, err := s.userRepo.FindByToken(token)
	if err != nil {
		return nil, errors.New("无效的订阅令牌")
	}
	if !user.Active {
		return nil, errors.New("订阅已停用，请联系管理员")
	}
	if user.ExpiresAt != nil && user.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("订阅已过期，请联系管理员")
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(s.settingSvc.Get(KeySubscriptionBaseURL)), "/")
	}
	subURL := baseURL + "/sub/" + token
	v2rayURL := subURL + "?format=v2ray"
	clashURL := subURL + "?format=clash"
	singboxURL := subURL + "?format=singbox"

	data := &SubscribePageData{
		Username:  user.Username,
		Active:    user.Active,
		ExpiresAt: user.ExpiresAt,
		SubURL:    subURL,
		AltSubURL: v2rayURL,
		SubLinks: []SubLink{
			{Label: "通用 Base64", Hint: "V2RayN / NekoRay / Shadowrocket", Format: "v2ray", URL: v2rayURL},
			{Label: "Clash 系", Hint: "Mihomo / Clash Verge / ClashX", Format: "clash", URL: clashURL},
			{Label: "Sing-box", Hint: "sing-box / Hiddify", Format: "singbox", URL: singboxURL},
		},
	}

	// 填充累计流量（查询失败时静默：信息页不应因辅助信息失败而 5xx）
	if totals, err := s.trafficRepo.ListTotalsByUserIDs([]uint{user.ID}); err == nil {
		if t, ok := totals[user.ID]; ok {
			data.TrafficUpBytes = t.UpBytes
			data.TrafficDownBytes = t.DownBytes
			data.TrafficTotalBytes = t.UpBytes + t.DownBytes
			data.TrafficUpHuman = formatBytes(t.UpBytes)
			data.TrafficDownHuman = formatBytes(t.DownBytes)
			data.TrafficTotalHuman = formatBytes(t.UpBytes + t.DownBytes)
			if !t.LastUpdatedAt.IsZero() {
				updatedAt := t.LastUpdatedAt
				data.TrafficLastUpdatedAt = &updatedAt
			}
		}
	}

	groupIDs := userGroupIDs(user.Groups)
	if len(groupIDs) == 0 {
		return data, nil
	}

	nodes, err := s.nodeRepo.FindHealthyByGroupIDs(groupIDs, true)
	if err != nil || len(nodes) == 0 {
		nodes, _ = s.nodeRepo.FindHealthyByGroupIDs(groupIDs, false)
	}
	sortNodesByName(nodes)

	for _, node := range nodes {
		node := node
		// 每个节点取第一个可用链接；节点无协议配置时也加入列表（link 为空）
		var firstLink string
		if profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID); err == nil {
			for _, key := range profileKeys {
				key := key
				if l := s.buildURI(user, &node, key.Profile, &key); l != "" {
					firstLink = l
					break
				}
			}
		}
		if firstLink == "" {
			continue
		}
		data.Nodes = append(data.Nodes, NodeLinkData{
			Name:   node.Name,
			Region: node.Region,
			Link:   firstLink,
		})
	}
	return data, nil
}

// GenerateClash 生成 Clash 格式订阅（YAML）
func (s *SubscribeService) GenerateClash(token string) (string, error) {
	user, err := s.userRepo.FindByToken(token)
	if err != nil {
		return "", errors.New("无效的订阅令牌")
	}
	if !user.Active {
		return "", errors.New("用户已禁用")
	}
	if user.ExpiresAt != nil && user.ExpiresAt.Before(time.Now()) {
		return "", errors.New("订阅已过期")
	}
	groupIDs := userGroupIDs(user.Groups)
	if len(groupIDs) == 0 {
		return "proxies: []\n", nil
	}

	nodes, err := s.nodeRepo.FindHealthyByGroupIDs(groupIDs, true)
	if err != nil || len(nodes) == 0 {
		nodes, _ = s.nodeRepo.FindHealthyByGroupIDs(groupIDs, false)
	}
	sortNodesByName(nodes)

	var proxies []string
	for _, node := range nodes {
		node := node
		profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
		if err != nil {
			continue
		}
		for _, key := range profileKeys {
			key := key
			entry := s.buildClashProxy(user, &node, key.Profile, &key)
			if entry != "" {
				proxies = append(proxies, entry)
			}
		}
	}

	if len(proxies) == 0 {
		return "proxies: []\n", nil
	}
	return "proxies:\n" + strings.Join(proxies, ""), nil
}

func sortNodesByName(nodes []entity.Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(nodes[i].Name))
		right := strings.ToLower(strings.TrimSpace(nodes[j].Name))
		if left == right {
			return nodes[i].ID < nodes[j].ID
		}
		return left < right
	})
}

func userGroupIDs(groups []entity.Group) []uint {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return slices.Compact(ids)
}

// buildClashProxy 根据协议生成 Clash proxy YAML 条目
func (s *SubscribeService) buildClashProxy(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	if profile == nil {
		return ""
	}
	name := s.buildRemark(node, user, string(profile.Protocol), "")
	addr := node.ConnectAddr()

	switch profile.Protocol {
	case types.ProtocolVlessReality:
		var ps types.VlessRealitySettings
		if profile.Settings != "" {
			_ = json.Unmarshal([]byte(profile.Settings), &ps)
		}
		var km types.RealityKeyMaterial
		if key.Settings != "" {
			_ = json.Unmarshal([]byte(key.Settings), &km)
		}
		sni := ps.SNI
		if sni == "" {
			sni = "www.microsoft.com"
		}
		fp := ps.Fingerprint
		if fp == "" {
			fp = "chrome"
		}
		shortID := ""
		if len(km.ShortIds) > 0 {
			shortID = km.ShortIds[0]
		}
		return fmt.Sprintf("  - name: %q\n    type: vless\n    server: %s\n    port: %d\n    uuid: %s\n    tls: true\n    servername: %s\n    flow: xtls-rprx-vision\n    client-fingerprint: %s\n    reality-opts:\n      public-key: %s\n      short-id: %s\n    network: tcp\n",
			name, addr, profile.Port, user.UUID, sni, fp, km.PublicKey, shortID)

	case types.ProtocolVlessWSTLS:
		var ps types.VlessWSTLSSettings
		if profile.Settings != "" {
			_ = json.Unmarshal([]byte(profile.Settings), &ps)
		}
		host := ps.Host
		if host == "" {
			host = addr
		}
		path := ps.Path
		if path == "" {
			path = "/"
		}
		return fmt.Sprintf("  - name: %q\n    type: vless\n    server: %s\n    port: %d\n    uuid: %s\n    tls: true\n    servername: %s\n    network: ws\n    ws-opts:\n      path: %s\n      headers:\n        Host: %s\n",
			name, addr, profile.Port, user.UUID, host, path, host)

	case types.ProtocolTrojan:
		var ps types.TrojanSettings
		if profile.Settings != "" {
			_ = json.Unmarshal([]byte(profile.Settings), &ps)
		}
		sni := ps.SNI
		if sni == "" {
			sni = addr
		}
		return fmt.Sprintf("  - name: %q\n    type: trojan\n    server: %s\n    port: %d\n    password: %s\n    sni: %s\n",
			name, addr, profile.Port, user.UUID, sni)

	case types.ProtocolHysteria2:
		var ps types.Hysteria2Settings
		if profile.Settings != "" {
			_ = json.Unmarshal([]byte(profile.Settings), &ps)
		}
		sni := ps.SNI
		if sni == "" {
			sni = addr
		}
		return fmt.Sprintf("  - name: %q\n    type: hysteria2\n    server: %s\n    port: %d\n    password: %s\n    sni: %s\n",
			name, addr, profile.Port, user.UUID, sni)
	}
	return ""
}

// decryptPrivateKey 解密 Reality 私钥（内部用）
func decryptPrivateKey(encryptedKey string) (string, error) {
	if encryptedKey == "" {
		return "", nil
	}
	plain, err := crypto.Decrypt(encryptedKey)
	if err != nil {
		return encryptedKey, nil // 兼容明文
	}
	return plain, nil
}

// formatBytes 将字节数格式化为人类可读字符串（二进制单位）
// 0 返回 "0 B"，与前端 lib/utils.ts 的 formatBytes 行为对齐
func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	switch {
	case i == 0:
		return fmt.Sprintf("%d %s", bytes, units[0])
	case value < 10:
		return fmt.Sprintf("%.2f %s", value, units[i])
	default:
		return fmt.Sprintf("%.1f %s", value, units[i])
	}
}
