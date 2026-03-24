package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// SubscribeService 订阅生成服务
type SubscribeService struct {
	userRepo *repository.UserRepository
	nodeRepo *repository.NodeRepository
}

func NewSubscribeService() *SubscribeService {
	return &SubscribeService{
		userRepo: repository.NewUserRepository(),
		nodeRepo: repository.NewNodeRepository(),
	}
}

// GenerateSubscription 生成用户订阅内容（base64 编码）
//
// 过滤链：
//  1. token → 查找激活用户
//  2. user.GroupID → 查找分组
//  3. 节点过滤：Active=true AND LastCheckOK=true（软剔除死节点）
//     若分组内无通过检测的节点，则降级返回所有 Active=true 节点（首次部署尚未检测时不返回空）
func (s *SubscribeService) GenerateSubscription(token string) (string, error) {
	user, err := s.userRepo.FindByToken(token)
	if err != nil {
		return "", errors.New("无效的订阅令牌")
	}
	if !user.Active {
		return "", errors.New("用户已禁用")
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
		link := buildVLESSURI(user, &node)
		if link != "" {
			links = append(links, link)
		}
	}

	content := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(content)), nil
}

// buildVLESSURI 构建单节点 VLESS Reality URI
//
// 格式：vless://<uuid>@<ip>:<port>?<params>#<remark>
//
// 参数说明：
//
//	encryption=none   不额外加密（Reality 已内置）
//	security=reality  启用 Reality
//	sni=<sni>         目标域名（伪装域名）
//	fp=chrome         TLS 指纹（浏览器指纹）
//	pbk=<publicKey>   Reality 服务端公钥（base64url）
//	sid=<shortId>     ShortID（可空）
//	type=tcp          传输层协议
//	flow=xtls-rprx-vision  XTLS flow（视觉模式，需服务端配合）
func buildVLESSURI(user *entity.User, node *entity.Node) string {
	if user.UUID == "" || node.IP == "" {
		return ""
	}

	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("security", "reality")
	params.Set("type", "tcp")
	params.Set("flow", "xtls-rprx-vision")
	params.Set("fp", "chrome")

	sni := node.SNI
	if sni == "" {
		sni = "www.microsoft.com"
	}
	params.Set("sni", sni)

	if node.PublicKey != "" {
		params.Set("pbk", node.PublicKey)
	}
	if node.ShortID != "" {
		params.Set("sid", node.ShortID)
	}

	port := node.Port
	if port == 0 {
		port = 443
	}

	// remark = "地区-节点名"，URL encode 处理特殊字符
	remark := node.Name
	if node.Region != "" {
		remark = node.Region + "-" + node.Name
	}

	uri := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID,
		node.IP,
		port,
		params.Encode(),
		url.QueryEscape(remark),
	)
	return uri
}
