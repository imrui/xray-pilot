package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"encoding/json"

	"github.com/imrui/xray-pilot/config"
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
}

func NewSubscribeService() *SubscribeService {
	return &SubscribeService{
		userRepo:    repository.NewUserRepository(),
		nodeRepo:    repository.NewNodeRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
	}
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
			link := buildURI(user, &node, key.Profile, &key)
			if link != "" {
				links = append(links, link)
			}
		}
	}

	content := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(content)), nil
}

// buildURI 根据协议类型分发 URI 构建
func buildURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
	if profile == nil {
		return ""
	}
	switch profile.Protocol {
	case types.ProtocolVlessReality:
		return buildVlessRealityURI(user, node, profile, key)
	case types.ProtocolVlessWSTLS:
		return buildVlessWSTLSURI(user, node, profile, key)
	case types.ProtocolTrojan:
		return buildTrojanURI(user, node, profile, key)
	case types.ProtocolHysteria2:
		return buildHysteria2URI(user, node, profile, key)
	}
	return ""
}

func buildVlessRealityURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
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
	if km.ShortID != "" {
		params.Set("sid", km.ShortID)
	}

	remark := buildRemark(node)
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.QueryEscape(remark))
}

func buildVlessWSTLSURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
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

	remark := buildRemark(node)
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.QueryEscape(remark))
}

func buildTrojanURI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
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

	remark := buildRemark(node)
	return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.QueryEscape(remark))
}

func buildHysteria2URI(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) string {
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

	remark := buildRemark(node)
	return fmt.Sprintf("hy2://%s@%s:%d?%s#%s",
		user.UUID, node.ConnectAddr(), profile.Port, params.Encode(), url.QueryEscape(remark))
}

// buildRemark 生成节点备注，支持 remark_format 配置
func buildRemark(node *entity.Node) string {
	format := config.Global.Subscription.RemarkFormat
	if format == "" {
		format = "{region}-{name}"
	}
	remark := format
	remark = strings.ReplaceAll(remark, "{region}", node.Region)
	remark = strings.ReplaceAll(remark, "{name}", node.Name)
	// 去除首尾多余的连字符
	remark = strings.Trim(remark, "-")
	if remark == "" {
		remark = node.Name
	}
	return remark
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
