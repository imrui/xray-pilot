package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

const (
	// 默认 token 有效期（10 分钟）；管理员可在 ttl_seconds 字段覆盖。
	defaultInstallTTL = 10 * time.Minute
	maxInstallTTL     = 24 * time.Hour

	// 节点装机脚本默认地址；可由 env XRAY_PILOT_BOOTSTRAP_URL 覆盖（v0.4.1+ 灰度切换用）。
	defaultBootstrapURL = "https://raw.githubusercontent.com/imrui/xray-pilot/main/scripts/node-bootstrap.sh"
)

// install token 鉴权相关 sentinel 错误
var (
	ErrInstallTokenNotFound    = errors.New("安装 token 不存在")
	ErrInstallTokenUsed        = errors.New("安装 token 已被使用")
	ErrInstallTokenExpired     = errors.New("安装 token 已过期")
	ErrInstallTokenIPMismatch  = errors.New("安装 token 来源 IP 与首次绑定不一致")
	ErrPanelSSHKeyMissing      = errors.New("panel 未配置 SSH 私钥（请在 config.yaml 设置 ssh.default_key_path 并保证 .pub 公钥存在）")
	ErrInstallNodeAlreadyExist = errors.New("同名节点已存在，请更换节点名后重新生成 token")
)

// nodeMeta 是 NodeInstallToken.NodeMeta 字段反序列化后的结构。
// 仅在 install 流程内部传递，未来字段叠加不会破坏接口。
type nodeMeta struct {
	Name    string `json:"name"`
	Region  string `json:"region,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Remark  string `json:"remark,omitempty"`
	SSHUser string `json:"ssh_user,omitempty"`
	SSHPort int    `json:"ssh_port,omitempty"`
}

// InstallService 节点一键接入流程的业务编排
type InstallService struct {
	tokenRepo *repository.NodeInstallTokenRepository
	nodeRepo  *repository.NodeRepository
	logRepo   *repository.LogRepository
}

func NewInstallService() *InstallService {
	return &InstallService{
		tokenRepo: repository.NewNodeInstallTokenRepository(),
		nodeRepo:  repository.NewNodeRepository(),
		logRepo:   repository.NewLogRepository(),
	}
}

// PanelPubKeyPath 返回 panel 公钥的绝对路径
// 约定：公钥 = 私钥路径 + ".pub"（ed25519 / rsa 默认形态）
func (s *InstallService) PanelPubKeyPath() string {
	priv := strings.TrimSpace(config.Global.SSH.DefaultKeyPath)
	if priv == "" {
		return ""
	}
	return priv + ".pub"
}

// ReadPanelPubKey 读取 panel 公钥内容
// 失败时返回 ErrPanelSSHKeyMissing，便于上层统一返回中文提示
func (s *InstallService) ReadPanelPubKey() (string, error) {
	path := s.PanelPubKeyPath()
	if path == "" {
		return "", ErrPanelSSHKeyMissing
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ErrPanelSSHKeyMissing
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", ErrPanelSSHKeyMissing
	}
	return content, nil
}

// CreateToken 生成一次性 token，并把 node_meta 持久化进 JSON 字段。
// 在生成前做关键前置校验：
//   - panel SSH 公钥可读
//   - 同名节点不存在（避免脚本注册时撞名）
func (s *InstallService) CreateToken(req *dto.CreateInstallTokenRequest, adminUsername string) (*dto.InstallTokenResponse, error) {
	if _, err := s.ReadPanelPubKey(); err != nil {
		return nil, err
	}
	if existing, err := s.nodeRepo.FindByName(req.Name); err == nil && existing != nil {
		return nil, ErrInstallNodeAlreadyExist
	}
	if strings.TrimSpace(req.PanelURL) == "" {
		return nil, errors.New("panel_url 不能为空")
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = defaultInstallTTL
	}
	if ttl > maxInstallTTL {
		ttl = maxInstallTTL
	}

	meta := nodeMeta{
		Name:    req.Name,
		Region:  req.Region,
		Owner:   req.Owner,
		Remark:  req.Remark,
		SSHUser: req.SSHUser,
		SSHPort: req.SSHPort,
	}
	if meta.SSHUser == "" {
		meta.SSHUser = "root"
	}
	if meta.SSHPort == 0 {
		meta.SSHPort = 22
	}
	metaBytes, err := json.Marshal(&meta)
	if err != nil {
		return nil, fmt.Errorf("序列化节点元数据失败: %w", err)
	}

	tokenStr, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("生成 token 失败: %w", err)
	}

	now := time.Now()
	t := &entity.NodeInstallToken{
		Token:          tokenStr,
		NodeMeta:       string(metaBytes),
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
		CreatedByAdmin: adminUsername,
	}
	if err := s.tokenRepo.Create(t); err != nil {
		return nil, fmt.Errorf("保存 token 失败: %w", err)
	}

	curl := buildInstallCurlCommand(strings.TrimRight(req.PanelURL, "/"), tokenStr)

	// 操作日志：actor 字段在 v0.5.0 落地，当前先把信息写进 message
	s.logRepo.Record(
		"install_token_create",
		fmt.Sprintf("node=%s", req.Name),
		true,
		fmt.Sprintf("actor=admin:%s ttl=%ds", adminUsername, int(ttl.Seconds())),
		0,
	)

	return s.toResponse(t, curl), nil
}

// AuthorizeToken 校验 token 是否处于可使用状态（未使用 / 未过期 / IP 匹配）。
// requireIP 非空时，token 已绑定 IP 则必须匹配。
// 在 panel-pubkey 端点首次调用时 requireIP 可传脚本来源 IP（绑定也由本方法触发）。
func (s *InstallService) AuthorizeToken(tokenStr, requireIP string) (*entity.NodeInstallToken, error) {
	t, err := s.tokenRepo.FindByToken(strings.TrimSpace(tokenStr))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInstallTokenNotFound
		}
		return nil, err
	}
	if t.IsUsed() {
		return nil, ErrInstallTokenUsed
	}
	if t.IsExpired(time.Now()) {
		return nil, ErrInstallTokenExpired
	}
	if requireIP != "" && t.UsedByIP != "" && t.UsedByIP != requireIP {
		return nil, ErrInstallTokenIPMismatch
	}
	return t, nil
}

// BindTokenIP 把首次访问 panel-pubkey 的脚本源 IP 锁定到 token
// 已绑定时不覆盖；要校验匹配先调 AuthorizeToken。
func (s *InstallService) BindTokenIP(t *entity.NodeInstallToken, ip string) error {
	if t.UsedByIP != "" || ip == "" {
		return nil
	}
	if err := s.tokenRepo.BindIP(t.ID, ip); err != nil {
		return err
	}
	t.UsedByIP = ip
	return nil
}

// RegisterNode 装机脚本回调创建 Node 记录 + 标记 token 已使用
// 调用前应 AuthorizeToken 校验过 IP 匹配。
func (s *InstallService) RegisterNode(t *entity.NodeInstallToken, sourceIP string, req *dto.RegisterNodeRequest) (*dto.RegisterNodeResponse, error) {
	var meta nodeMeta
	if err := json.Unmarshal([]byte(t.NodeMeta), &meta); err != nil {
		return nil, fmt.Errorf("解析 token 节点元数据失败: %w", err)
	}

	// 公网 IP 优先用脚本上报的 PublicIP，缺失则回退源 IP（兼容脚本 ipify 调用失败）
	ip := strings.TrimSpace(req.PublicIP)
	if ip == "" || ip == "unknown" {
		ip = sourceIP
	}

	now := time.Now()
	node := &entity.Node{
		Name:           meta.Name,
		Region:         meta.Region,
		Owner:          meta.Owner,
		Remark:         meta.Remark,
		IP:             ip,
		SSHUser:        meta.SSHUser,
		SSHPort:        meta.SSHPort,
		Active:         true,
		SyncStatus:     entity.SyncStatusPending,
		ConnectionMode: "ssh",
		RegisteredAt:   &now,
		XrayVersion:    strings.TrimSpace(req.XrayVersion),
	}
	if node.SSHPort == 0 {
		node.SSHPort = 22
	}
	if node.SSHUser == "" {
		node.SSHUser = "root"
	}

	if err := s.nodeRepo.Create(node); err != nil {
		return nil, fmt.Errorf("创建节点失败: %w", err)
	}

	if err := s.tokenRepo.MarkUsed(t.ID, node.ID, now); err != nil {
		// 并发场景下别的并行 register 抢先了；删除刚创建的节点防止脏数据
		_ = s.nodeRepo.Delete(node.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInstallTokenUsed
		}
		return nil, err
	}

	short := tokenShortID(t.Token)
	s.logRepo.Record(
		"install_node_register",
		fmt.Sprintf("node=%s id=%d", node.Name, node.ID),
		true,
		fmt.Sprintf("actor=system:install-token:%s ip=%s xray=%s kernel=%s distro=%s",
			short, ip, req.XrayVersion, req.Kernel, req.Distro),
		0,
	)

	return &dto.RegisterNodeResponse{NodeID: node.ID, Name: node.Name}, nil
}

// ListActive 列出活跃 token；不带 curl 命令（避免反向暴露）
func (s *InstallService) ListActive() ([]dto.InstallTokenResponse, error) {
	tokens, err := s.tokenRepo.ListActive(time.Now())
	if err != nil {
		return nil, err
	}
	out := make([]dto.InstallTokenResponse, 0, len(tokens))
	for i := range tokens {
		out = append(out, *s.toResponse(&tokens[i], ""))
	}
	return out, nil
}

// FindByToken 单点查询（前端轮询用）；按 token 字符串而非 ID。
// 返回未使用 / 已使用 / 过期 三种状态都需要管理员可见，因此不做状态过滤。
func (s *InstallService) FindByToken(tokenStr string) (*dto.InstallTokenResponse, error) {
	t, err := s.tokenRepo.FindByToken(tokenStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInstallTokenNotFound
		}
		return nil, err
	}
	return s.toResponse(t, ""), nil
}

// Delete 管理员主动撤销
func (s *InstallService) Delete(id uint, adminUsername string) error {
	if err := s.tokenRepo.Delete(id); err != nil {
		return err
	}
	s.logRepo.Record(
		"install_token_delete",
		fmt.Sprintf("token_id=%d", id),
		true,
		fmt.Sprintf("actor=admin:%s", adminUsername),
		0,
	)
	return nil
}

// CleanupExpired 后台调度器调用：清理已过期且未使用的 token
func (s *InstallService) CleanupExpired() (int64, error) {
	return s.tokenRepo.DeleteExpired(time.Now())
}

func (s *InstallService) toResponse(t *entity.NodeInstallToken, curl string) *dto.InstallTokenResponse {
	resp := &dto.InstallTokenResponse{
		ID:        t.ID,
		Token:     t.Token,
		ExpiresAt: t.ExpiresAt,
		Used:      t.IsUsed(),
		UsedByIP:  t.UsedByIP,
	}
	if t.NodeID != nil {
		resp.NodeID = t.NodeID
	}
	var meta nodeMeta
	if err := json.Unmarshal([]byte(t.NodeMeta), &meta); err == nil {
		resp.NodeName = meta.Name
	}
	if curl != "" {
		resp.CurlCommand = curl
	}
	return resp
}

// randomToken 生成 length 字节的随机 token（hex 编码）
func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// tokenShortID 取 token 前 8 位作为审计日志中的简短标识
func tokenShortID(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

func buildInstallCurlCommand(panelURL, token string) string {
	bootstrapURL := os.Getenv("XRAY_PILOT_BOOTSTRAP_URL")
	if bootstrapURL == "" {
		bootstrapURL = defaultBootstrapURL
	}
	return fmt.Sprintf(
		"curl -fsSL %s | sudo PANEL_URL=%s INSTALL_TOKEN=%s bash",
		bootstrapURL, panelURL, token,
	)
}
