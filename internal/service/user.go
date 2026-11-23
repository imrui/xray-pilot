package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type UserService struct {
	userRepo    *repository.UserRepository
	trafficRepo *repository.TrafficRepository
	liveSync    *LiveSyncService
}

func NewUserService() *UserService {
	return &UserService{
		userRepo:    repository.NewUserRepository(),
		trafficRepo: repository.NewTrafficRepository(),
		liveSync:    NewLiveSyncService(),
	}
}

func (s *UserService) applyAccessChange(change repository.UserAccessChange, action string) {
	if len(change.NodeIDs) > 0 && !change.Renamed && s.liveSync.Enabled() {
		s.liveSync.applyOverNodes(change.NodeIDs, action, change.Username)
	}
}

func (s *UserService) Create(req *dto.CreateUserRequest, baseURL string) (*dto.UserResponse, error) {
	relay, err := relayID(req.RelaySubscriptionID)
	if err != nil {
		return nil, err
	}
	var feishuBoundAt *time.Time
	if req.FeishuOpenID != "" || req.FeishuUnionID != "" || req.FeishuChatID != "" {
		now := time.Now()
		feishuBoundAt = &now
	}

	user := &entity.User{
		RelaySubscriptionID: relay,
		Username:            strings.TrimSpace(req.Username),
		RealName:            req.RealName,
		Remark:              req.Remark,
		FeishuEnabled:       req.FeishuEnabled,
		FeishuEmail:         strings.ToLower(strings.TrimSpace(req.FeishuEmail)),
		UUID:                uuid.NewString(),
		Token:               uuid.NewString(),
		Active:              true,
		ExpiresAt:           req.ExpiresAt,
		FeishuOpenID:        req.FeishuOpenID,
		FeishuUnionID:       req.FeishuUnionID,
		FeishuChatID:        req.FeishuChatID,
		FeishuBoundAt:       feishuBoundAt,
	}
	if user.Username == "" {
		return nil, errors.New("用户名不能为空")
	}
	var nodes []uint
	if err := repository.AuthorizationTransaction(func(tx *gorm.DB) error {
		now := time.Now()
		if user.IsExpired(now) {
			user.ExpiredSweptAt = &now
		}
		if err := tx.Create(user).Error; err != nil {
			return fmt.Errorf("创建用户失败: %w", relayUserError(err))
		}
		if err := s.userRepo.ReplaceGroupsTx(tx, user, req.GroupIDs); err != nil {
			return fmt.Errorf("保存用户分组失败: %w", err)
		}
		if user.EffectiveActive(now) {
			var err error
			nodes, err = repository.UserConfigNodes(tx, user.ID)
			if err != nil {
				return err
			}
			if len(nodes) > 0 {
				return repository.InvalidateConfigs(tx, nodes)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	created, err := s.userRepo.FindByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("读取用户失败: %w", err)
	}
	if len(nodes) > 0 && s.liveSync.Enabled() {
		s.liveSync.AddUserLive(nodes, created.Username, created.UUID)
	}
	return s.toResponse(created, baseURL, currentRelayPrefix()), nil
}

func (s *UserService) Update(id uint, req *dto.UpdateUserRequest, baseURL string) (*dto.UserResponse, error) {
	change, err := s.userRepo.ChangeAccess(id, func(tx *gorm.DB, u *entity.User) error {
		updates := map[string]any{}
		if req.RelaySubscriptionID != nil {
			v, err := relayID(*req.RelaySubscriptionID)
			if err != nil {
				return err
			}
			updates["relay_subscription_id"] = v
		}
		if req.Username != nil {
			name := strings.TrimSpace(*req.Username)
			if name == "" {
				return errors.New("用户名不能为空")
			}
			updates["username"] = name
		}
		if req.RealName != nil {
			updates["real_name"] = *req.RealName
		}
		if req.Remark != nil {
			updates["remark"] = *req.Remark
		}
		if req.Active != nil {
			updates["active"] = *req.Active
		}
		if req.FeishuEnabled != nil {
			updates["feishu_enabled"] = *req.FeishuEnabled
		}
		if req.FeishuEmail != nil {
			updates["feishu_email"] = strings.ToLower(strings.TrimSpace(*req.FeishuEmail))
		}
		if req.FeishuOpenID != nil || req.FeishuUnionID != nil || req.FeishuChatID != nil {
			open, union, chat := u.FeishuOpenID, u.FeishuUnionID, u.FeishuChatID
			if req.FeishuOpenID != nil {
				open = *req.FeishuOpenID
				updates["feishu_open_id"] = open
			}
			if req.FeishuUnionID != nil {
				union = *req.FeishuUnionID
				updates["feishu_union_id"] = union
			}
			if req.FeishuChatID != nil {
				chat = *req.FeishuChatID
				updates["feishu_chat_id"] = chat
			}
			if open == "" && union == "" && chat == "" {
				updates["feishu_bound_at"] = nil
			} else if u.FeishuBoundAt == nil || open != u.FeishuOpenID || union != u.FeishuUnionID || chat != u.FeishuChatID {
				updates["feishu_bound_at"] = time.Now()
			}
		}
		if req.ExpiresAt != nil {
			until, err := parseOptionalTime(req.ExpiresAt)
			if err != nil {
				return fmt.Errorf("解析过期时间失败: %w", err)
			}
			if (until == nil) != (u.ExpiresAt == nil) || (until != nil && !until.Equal(*u.ExpiresAt)) {
				updates["expires_at"] = until
				now := time.Now()
				if !u.IsExpired(now) || until == nil || until.After(now) {
					updates["expired_swept_at"] = nil
					updates["expiry_queued"] = false
				}
			}
		}
		if req.GroupIDs != nil {
			ids, err := parseOptionalUintSlice(req.GroupIDs)
			if err != nil {
				return fmt.Errorf("解析分组失败: %w", err)
			}
			if !slices.Equal(ids, extractGroupIDs(u.Groups)) {
				if err := s.userRepo.ReplaceGroupsTx(tx, u, ids); err != nil {
					return fmt.Errorf("更新用户分组失败: %w", err)
				}
			}
		}
		if len(updates) == 0 {
			return nil
		}
		return relayUserError(tx.Model(&entity.User{}).Where("id = ?", id).Updates(updates).Error)
	})
	if err != nil {
		return nil, userEditError(err)
	}
	s.applyAccessChange(change, "更新用户")
	return s.toResponse(change.User, baseURL, currentRelayPrefix()), nil
}

func userEditError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("用户不存在")
	}
	return err
}

func (s *UserService) Delete(id uint) error {
	change, err := s.userRepo.DeleteAccess(id)
	if err != nil {
		return err
	}
	s.applyAccessChange(change, "摘除用户")
	return nil
}

func (s *UserService) ToggleActive(id uint) error {
	change, err := s.userRepo.ChangeAccess(id, func(tx *gorm.DB, u *entity.User) error {
		return tx.Model(&entity.User{}).Where("id = ?", u.ID).Update("active", !u.Active).Error
	})
	if err != nil {
		return userEditError(err)
	}
	s.applyAccessChange(change, "用户启停")
	return nil
}

// SweepResult 一次过期摘除的汇总
type SweepResult struct {
	Swept     []string // 已处理的用户名
	LiveApply bool     // 本次是否先尝试 gRPC 对齐运行态
}

// SweepExpired 把已过期但仍启用的用户从节点运行时摘除（给调度器周期调用）。
//   - live-apply on：按节点热更新，再确认磁盘及运行态撤权
//   - live-apply off：标记漂移并执行完整同步；无法确认当前授权时在线停止保护
//
// 处理过的用户打 ExpiredSweptAt 标记，不重复扫；续期会清标记。
// 订阅层的 403 拦截与本方法互补：前者挡新取订阅，后者撤销节点账号。
func (s *UserService) SweepExpired(now time.Time) (*SweepResult, error) {
	targets, swept, err := repository.PrepareExpirySweep(now)
	if err != nil {
		return nil, err
	}
	result := &SweepResult{Swept: swept, LiveApply: s.liveSync.Enabled()}
	for len(targets) > 0 {
		// 同一节点只对齐一次权限，每张回执绑定登记时的 Ticket。
		id := targets[0].NodeID
		var tickets []string
		for len(targets) > 0 && targets[0].NodeID == id {
			tickets = append(tickets, targets[0].Ticket)
			targets = targets[1:]
		}
		if !s.liveSync.confirmExpiredNode(id, tickets, result.LiveApply) {
			continue
		}
		completed, err := repository.ConfirmExpiryTargets(tickets, now)
		if err != nil {
			return result, err
		}
		result.Swept = append(result.Swept, completed...)
	}
	return result, nil
}

func (s *UserService) List(page, pageSize int, baseURL string, filter repository.UserListFilter) (*dto.UserListResponse, error) {
	users, total, err := s.userRepo.List(page, pageSize, filter)
	if err != nil {
		return nil, err
	}
	// 一次性 join 当页用户的累计流量，避免 N+1
	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	totals, _ := s.trafficRepo.ListTotalsByUserIDs(ids) // 失败时空 map，不阻塞主流程

	baseRelay := currentRelayPrefix()
	result := make([]dto.UserResponse, 0, len(users))
	for i := range users {
		resp := s.toResponse(&users[i], baseURL, baseRelay)
		if t, ok := totals[users[i].ID]; ok {
			resp.TrafficUpBytes = t.UpBytes
			resp.TrafficDownBytes = t.DownBytes
			if !t.LastUpdatedAt.IsZero() {
				resp.TrafficLastUpdatedAt = t.LastUpdatedAt.Format(time.RFC3339)
			}
		}
		result = append(result, *resp)
	}
	return &dto.UserListResponse{Total: total, List: result, RelayEnabled: baseRelay != "", RelayPrefix: baseRelay}, nil
}

// ResetUUID 仅对持有该账号有效授权的节点换证。
func (s *UserService) ResetUUID(id uint, baseURL string) (*dto.UserResponse, error) {
	change, err := s.userRepo.ChangeAccess(id, func(tx *gorm.DB, u *entity.User) error {
		return tx.Model(&entity.User{}).Where("id = ?", u.ID).Update("uuid", uuid.NewString()).Error
	})
	if err != nil {
		return nil, userEditError(err)
	}
	s.applyAccessChange(change, "重置用户 UUID")
	return s.toResponse(change.User, baseURL, currentRelayPrefix()), nil
}

// ResetToken 重置用户订阅 Token
func (s *UserService) ResetToken(id uint, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	newToken := uuid.NewString()
	if err := s.userRepo.UpdateToken(id, newToken); err != nil {
		return nil, fmt.Errorf("重置 Token 失败: %w", err)
	}
	user.Token = newToken
	return s.toResponse(user, baseURL, currentRelayPrefix()), nil
}

func (s *UserService) toResponse(u *entity.User, baseURL, baseRelay string) *dto.UserResponse {
	groupIDs := extractGroupIDs(u.Groups)
	groupNames := extractGroupNames(u.Groups)
	groups := make([]dto.UserGroupSummary, 0, len(u.Groups))
	for _, group := range u.Groups {
		groups = append(groups, dto.UserGroupSummary{
			ID:   group.ID,
			Name: group.Name,
		})
	}

	resp := &dto.UserResponse{
		RelaySubscriptionID: u.RelaySubscriptionID, RelaySubscriptionURL: relayURL(baseRelay, u),
		ID:            u.ID,
		Username:      u.Username,
		RealName:      u.RealName,
		GroupIDs:      groupIDs,
		GroupNames:    groupNames,
		Groups:        groups,
		Active:        u.Active,
		Remark:        u.Remark,
		SubscribeURL:  fmt.Sprintf("%s/sub/%s", baseURL, u.Token),
		FeishuEnabled: u.FeishuEnabled,
		FeishuEmail:   u.FeishuEmail,
		FeishuOpenID:  u.FeishuOpenID,
		FeishuUnionID: u.FeishuUnionID,
		FeishuChatID:  u.FeishuChatID,
		CreatedAt:     u.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     u.UpdatedAt.Format(time.RFC3339),
	}
	if u.ExpiresAt != nil {
		resp.ExpiresAt = u.ExpiresAt.Format(time.RFC3339)
	}
	if u.FeishuBoundAt != nil {
		resp.FeishuBoundAt = u.FeishuBoundAt.Format(time.RFC3339)
	}
	return resp
}

func parseOptionalUintSlice(raw json.RawMessage) ([]uint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values []uint
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return uniqueSortedIDs(values), nil
}

func parseOptionalTime(raw json.RawMessage) (*time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("unsupported time format: %s", value)
}

func extractGroupIDs(groups []entity.Group) []uint {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return uniqueSortedIDs(ids)
}

func extractGroupNames(groups []entity.Group) []string {
	if len(groups) == 0 {
		return nil
	}
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func uniqueSortedIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	cloned := append([]uint(nil), ids...)
	slices.Sort(cloned)
	return slices.Compact(cloned)
}
