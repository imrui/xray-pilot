package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"github.com/imrui/xray-pilot/pkg/types"
)

// ProfileService 协议接入配置管理服务
type ProfileService struct {
	profileRepo *repository.InboundProfileRepository
	nodeRepo    *repository.NodeRepository
}

func NewProfileService() *ProfileService {
	return &ProfileService{
		profileRepo: repository.NewInboundProfileRepository(),
		nodeRepo:    repository.NewNodeRepository(),
	}
}

func (s *ProfileService) Create(req *dto.CreateProfileRequest) (*dto.ProfileResponse, error) {
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	p := &entity.InboundProfile{
		Name:     req.Name,
		Protocol: req.Protocol,
		Port:     req.Port,
		Settings: normalizeSettingsJSON(req.Settings),
		Active:   active,
		Remark:   req.Remark,
	}
	if err := s.profileRepo.Create(p); err != nil {
		return nil, fmt.Errorf("创建协议配置失败: %w", err)
	}
	_ = s.nodeRepo.MarkAllDrifted()
	return toProfileResponse(p), nil
}

func (s *ProfileService) Update(id uint, req *dto.UpdateProfileRequest) (*dto.ProfileResponse, error) {
	p, err := s.profileRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("协议配置不存在")
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		p.Name = *req.Name
	}
	if req.Protocol != nil && *req.Protocol != "" {
		p.Protocol = *req.Protocol
	}
	if req.Port != nil && *req.Port != 0 {
		p.Port = *req.Port
	}
	if len(req.Settings) > 0 {
		p.Settings = normalizeSettingsJSON(req.Settings)
	}
	if req.Active != nil {
		p.Active = *req.Active
	}
	if req.Remark != nil {
		p.Remark = *req.Remark
	}
	if err := s.profileRepo.Update(p); err != nil {
		return nil, err
	}
	_ = s.nodeRepo.MarkAllDrifted()
	return toProfileResponse(p), nil
}

func (s *ProfileService) Delete(id uint) error {
	if err := s.profileRepo.Delete(id); err != nil {
		return err
	}
	return s.nodeRepo.MarkAllDrifted()
}

func (s *ProfileService) List(page, pageSize int) ([]dto.ProfileResponse, int64, error) {
	profiles, total, err := s.profileRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.ProfileResponse, 0, len(profiles))
	for i := range profiles {
		result = append(result, *toProfileResponse(&profiles[i]))
	}
	return result, total, nil
}

func (s *ProfileService) GetByID(id uint) (*dto.ProfileResponse, error) {
	p, err := s.profileRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("协议配置不存在")
	}
	return toProfileResponse(p), nil
}

// UpsertNodeKey 创建或更新节点密钥材料，对 Reality 私钥进行 AES-GCM 加密
func (s *ProfileService) UpsertNodeKey(nodeID, profileID uint, req *dto.UpsertNodeKeyRequest) (*dto.NodeKeyResponse, error) {
	profile, err := s.profileRepo.FindByID(profileID)
	if err != nil {
		return nil, errors.New("协议配置不存在")
	}

	settingsJSON := normalizeSettingsJSON(req.Settings)
	// 对 Reality 私钥加密存储
	if profile.Protocol == types.ProtocolVlessReality {
		settingsJSON, err = encryptRealityKey(settingsJSON)
		if err != nil {
			return nil, fmt.Errorf("加密私钥失败: %w", err)
		}
	}

	key := &entity.NodeProfileKey{
		NodeID:    nodeID,
		ProfileID: profileID,
		Settings:  settingsJSON,
	}
	if err := s.profileRepo.UpsertKey(key); err != nil {
		if errors.Is(err, repository.ErrNodeKeyLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("保存节点密钥失败: %w", err)
	}
	_ = s.nodeRepo.MarkAllDrifted()
	return toNodeKeyResponse(key), nil
}

// GetNodeKeys 获取节点关联的所有协议密钥（编辑场景返回可编辑明文）
func (s *ProfileService) GetNodeKeys(nodeID uint) ([]dto.NodeKeyResponse, error) {
	keys, err := s.profileRepo.FindKeysForNode(nodeID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.NodeKeyResponse, 0, len(keys))
	for i := range keys {
		result = append(result, *toNodeKeyResponse(&keys[i]))
	}
	return result, nil
}

// DeleteNodeKey 删除节点与协议的关联密钥
func (s *ProfileService) DeleteNodeKey(nodeID, profileID uint) error {
	if err := s.profileRepo.DeleteKey(nodeID, profileID); err != nil {
		if errors.Is(err, repository.ErrNodeKeyLocked) {
			return err
		}
		return err
	}
	return s.nodeRepo.MarkAllDrifted()
}

// KeygenForNode 为节点+协议自动生成并存储密钥对（仅支持 vless-reality）
func (s *ProfileService) KeygenForNode(nodeID, profileID uint) (*dto.NodeKeyResponse, error) {
	profile, err := s.profileRepo.FindByID(profileID)
	if err != nil {
		return nil, errors.New("协议配置不存在")
	}
	if profile.Protocol != types.ProtocolVlessReality {
		return nil, errors.New("仅支持 vless-reality 协议自动生成密钥")
	}

	privKey, pubKey, err := xray.GenerateX25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("生成密钥对失败: %w", err)
	}
	shortIDs := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		shortID, genErr := xray.GenerateShortID()
		if genErr != nil {
			return nil, fmt.Errorf("生成 short_id 失败: %w", genErr)
		}
		shortIDs = append(shortIDs, shortID)
	}

	// 加密私钥后存储
	encPrivKey, err := crypto.Encrypt(privKey)
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}

	km := types.RealityKeyMaterial{
		PrivateKey: encPrivKey,
		PublicKey:  pubKey,
		ShortIds:   shortIDs,
	}
	settingsJSON, err := json.Marshal(km)
	if err != nil {
		return nil, fmt.Errorf("序列化密钥材料失败: %w", err)
	}

	key := &entity.NodeProfileKey{
		NodeID:    nodeID,
		ProfileID: profileID,
		Settings:  string(settingsJSON),
	}
	if err := s.profileRepo.UpsertKey(key); err != nil {
		if errors.Is(err, repository.ErrNodeKeyLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("保存节点密钥失败: %w", err)
	}
	_ = s.nodeRepo.MarkAllDrifted()
	return toNodeKeyResponse(key), nil
}

// SetNodeKeyLocked 更新节点协议锁定状态
func (s *ProfileService) SetNodeKeyLocked(nodeID, profileID uint, locked bool) error {
	keys, err := s.profileRepo.FindKeysForNode(nodeID)
	if err != nil {
		return err
	}
	found := false
	for _, key := range keys {
		if key.ProfileID == profileID {
			found = true
			break
		}
	}
	if !found {
		return errors.New("节点协议不存在，请先保存节点密钥")
	}
	return s.profileRepo.SetKeyLocked(nodeID, profileID, locked)
}

// ---- 内部工具 ----

func toProfileResponse(p *entity.InboundProfile) *dto.ProfileResponse {
	var settings json.RawMessage
	if p.Settings != "" {
		settings = json.RawMessage(p.Settings)
	}
	return &dto.ProfileResponse{
		ID:        p.ID,
		Name:      p.Name,
		Protocol:  p.Protocol,
		Port:      p.Port,
		Settings:  settings,
		Active:    p.Active,
		Remark:    p.Remark,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
}

// toNodeKeyResponse 返回节点密钥响应；Reality 私钥在返回前解密，供编辑界面直接使用
func toNodeKeyResponse(k *entity.NodeProfileKey) *dto.NodeKeyResponse {
	var settings json.RawMessage
	decoded := decryptRealityKey(k.Settings)
	if decoded != "" {
		settings = json.RawMessage(decoded)
	}
	return &dto.NodeKeyResponse{
		NodeID:    k.NodeID,
		ProfileID: k.ProfileID,
		Settings:  settings,
		Locked:    k.Locked,
		CreatedAt: k.CreatedAt.Format(time.RFC3339),
		UpdatedAt: k.UpdatedAt.Format(time.RFC3339),
	}
}

// normalizeSettingsJSON 规范化 settings 存储格式
// 若前端将 JSON 作为字符串值发送（二次编码），先展开为 JSON 对象字符串
func normalizeSettingsJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 如果是 JSON string（首字节为引号），展开内层字符串
	if raw[0] == '"' {
		var unwrapped string
		if err := json.Unmarshal(raw, &unwrapped); err == nil {
			return unwrapped
		}
	}
	return string(raw)
}

// decryptRealityKey 将 settings JSON 中的 private_key 从密文还原为明文。
// 若不是 Reality 密钥材料或解密失败，则返回原始 settings，避免影响非加密协议。
func decryptRealityKey(settings string) string {
	if settings == "" {
		return settings
	}

	var km types.RealityKeyMaterial
	if err := json.Unmarshal([]byte(settings), &km); err != nil {
		return settings
	}
	if km.PrivateKey == "" {
		return settings
	}
	plain, err := crypto.Decrypt(km.PrivateKey)
	if err != nil {
		return settings
	}
	km.PrivateKey = plain
	decoded, err := json.Marshal(km)
	if err != nil {
		return settings
	}
	return string(decoded)
}

// encryptRealityKey 对 RealityKeyMaterial JSON 中的 private_key 字段加密
func encryptRealityKey(settingsJSON string) (string, error) {
	var km types.RealityKeyMaterial
	if err := json.Unmarshal([]byte(settingsJSON), &km); err != nil {
		return "", fmt.Errorf("解析密钥材料失败: %w", err)
	}
	if km.PrivateKey == "" {
		return settingsJSON, nil
	}
	encKey, err := crypto.Encrypt(km.PrivateKey)
	if err != nil {
		return "", err
	}
	km.PrivateKey = encKey
	data, err := json.Marshal(km)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
