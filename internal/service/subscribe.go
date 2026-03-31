package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
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
	settingSvc  *SettingService
}

func NewSubscribeService() *SubscribeService {
	return &SubscribeService{
		userRepo:    repository.NewUserRepository(),
		nodeRepo:    repository.NewNodeRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
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
	if user.GroupID == nil {
		return base64.StdEncoding.EncodeToString([]byte("")), nil
	}

	// 优先返回健康节点，无健康节点时降级返回全部激活节点
	nodes, err := s.nodeRepo.FindHealthyByGroupID(*user.GroupID, true)
	if err != nil {
		return "", fmt.Errorf("查询节点失败: %w", err)
	}
	if len(nodes) == 0 {
		nodes, err = s.nodeRepo.FindHealthyByGroupID(*user.GroupID, false)
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
	SubURL    string
	AltSubURL string
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
	altURL := subURL + "?sub=1"

	data := &SubscribePageData{
		Username:  user.Username,
		Active:    user.Active,
		ExpiresAt: user.ExpiresAt,
		SubURL:    subURL,
		AltSubURL: altURL,
	}

	if user.GroupID == nil {
		return data, nil
	}

	nodes, err := s.nodeRepo.FindHealthyByGroupID(*user.GroupID, true)
	if err != nil || len(nodes) == 0 {
		nodes, _ = s.nodeRepo.FindHealthyByGroupID(*user.GroupID, false)
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
	if user.GroupID == nil {
		return "proxies: []\n", nil
	}

	nodes, err := s.nodeRepo.FindHealthyByGroupID(*user.GroupID, true)
	if err != nil || len(nodes) == 0 {
		nodes, _ = s.nodeRepo.FindHealthyByGroupID(*user.GroupID, false)
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
