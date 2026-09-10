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
	nodeRepo    *repository.NodeRepository
	trafficRepo *repository.TrafficRepository
	liveSync    *LiveSyncService
}

func NewUserService() *UserService {
	return &UserService{
		userRepo:    repository.NewUserRepository(),
		nodeRepo:    repository.NewNodeRepository(),
		trafficRepo: repository.NewTrafficRepository(),
		liveSync:    NewLiveSyncService(),
	}
}

// nodeSetIf 用户激活时返回其节点集，未激活返回空（未激活用户不应出现在任何节点运行时）。
func nodeSetIf(active bool, ids []uint) []uint {
	if !active {
		return nil
	}
	return ids
}

// nodeIDsDiff 返回 a - b（在 a 中但不在 b 中的节点 ID）。
func nodeIDsDiff(a, b []uint) []uint {
	if len(a) == 0 {
		return nil
	}
	set := make(map[uint]struct{}, len(b))
	for _, id := range b {
		set[id] = struct{}{}
	}
	out := make([]uint, 0)
	for _, id := range a {
		if _, ok := set[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// applyUpdateLive 用户更新后的增量 live-apply：基于「旧/新 期望节点集」差集做增删，
// 只触碰真正变化的节点（加入新分组的节点、移出分组的节点、启停切换、过期/续期）。
// 期望集用 EffectiveActive（启用且未过期），与配置生成的用户过滤语义一致。
// 用户名(email)变更涉及跨节点 email 改动，逻辑复杂且罕见，回退全量 restart 同步。
func (s *UserService) applyUpdateLive(u *entity.User, oldNodeIDs []uint, oldEffectiveActive bool, oldUsername string) {
	if u.Username != oldUsername {
		_ = s.nodeRepo.MarkAllDrifted()
		return
	}
	desiredOld := nodeSetIf(oldEffectiveActive, oldNodeIDs)
	desiredNew := nodeSetIf(u.EffectiveActive(time.Now()), s.userNodeIDs(u))
	if toAdd := nodeIDsDiff(desiredNew, desiredOld); len(toAdd) > 0 {
		s.liveSync.AddUserLive(toAdd, u.Username, u.UUID)
	}
	if toRemove := nodeIDsDiff(desiredOld, desiredNew); len(toRemove) > 0 {
		s.liveSync.RemoveUserLive(toRemove, u.Username)
	}
}

// userNodeIDs 返回用户所有分组覆盖到的激活节点 ID 集合（不做健康过滤，尽力对每个节点 live-apply）。
// 用于用户增删时确定要即时操作的节点范围。
func (s *UserService) userNodeIDs(user *entity.User) []uint {
	groupIDs := extractGroupIDs(user.Groups)
	if len(groupIDs) == 0 {
		return nil
	}
	nodes, err := s.nodeRepo.FindHealthyByGroupIDs(groupIDs, false)
	if err != nil {
		return nil
	}
	ids := make([]uint, 0, len(nodes))
	for i := range nodes {
		ids = append(ids, nodes[i].ID)
	}
	return ids
}

func (s *UserService) Create(req *dto.CreateUserRequest, baseURL string) (*dto.UserResponse, error) {
	var feishuBoundAt *time.Time
	if req.FeishuOpenID != "" || req.FeishuUnionID != "" || req.FeishuChatID != "" {
		now := time.Now()
		feishuBoundAt = &now
	}

	user := &entity.User{
		Username:      strings.TrimSpace(req.Username),
		RealName:      req.RealName,
		Remark:        req.Remark,
		FeishuEnabled: req.FeishuEnabled,
		FeishuEmail:   strings.ToLower(strings.TrimSpace(req.FeishuEmail)),
		UUID:          uuid.NewString(),
		Token:         uuid.NewString(),
		Active:        true,
		ExpiresAt:     req.ExpiresAt,
		FeishuOpenID:  req.FeishuOpenID,
		FeishuUnionID: req.FeishuUnionID,
		FeishuChatID:  req.FeishuChatID,
		FeishuBoundAt: feishuBoundAt,
	}
	if user.Username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if err := repository.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return fmt.Errorf("创建用户失败: %w", err)
		}
		if err := s.userRepo.ReplaceGroupsTx(tx, user, req.GroupIDs); err != nil {
			return fmt.Errorf("保存用户分组失败: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	created, err := s.userRepo.FindByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("读取用户失败: %w", err)
	}
	if s.liveSync.Enabled() {
		// 创建即过期的用户不在任何节点运行时（配置生成已过滤），无需 live-apply
		if created.EffectiveActive(time.Now()) {
			s.liveSync.AddUserLive(s.userNodeIDs(created), created.Username, created.UUID)
		}
	} else {
		_ = s.nodeRepo.MarkAllDrifted()
	}
	return s.toResponse(created, baseURL), nil
}

func (s *UserService) Update(id uint, req *dto.UpdateUserRequest, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	// live-apply：在改动前捕获旧的节点集 / 活跃态 / 用户名，用于结束后做增量对账
	liveOn := s.liveSync.Enabled()
	var (
		oldNodeIDs         []uint
		oldEffectiveActive bool
		oldUsername        string
	)
	if liveOn {
		oldNodeIDs = s.userNodeIDs(user)
		oldEffectiveActive = user.EffectiveActive(time.Now())
		oldUsername = user.Username
	}
	shouldMarkSync := false
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			return nil, fmt.Errorf("用户名不能为空")
		}
		if user.Username != username {
			shouldMarkSync = true
		}
		user.Username = username
	}
	if req.RealName != nil {
		user.RealName = *req.RealName
	}
	if req.GroupIDs != nil {
		groupIDs, err := parseOptionalUintSlice(req.GroupIDs)
		if err != nil {
			return nil, fmt.Errorf("解析分组失败: %w", err)
		}
		currentGroupIDs := extractGroupIDs(user.Groups)
		if !slices.Equal(currentGroupIDs, groupIDs) {
			shouldMarkSync = true
		}
	}
	if req.Remark != nil {
		user.Remark = *req.Remark
	}
	if req.FeishuEnabled != nil {
		user.FeishuEnabled = *req.FeishuEnabled
	}
	if req.FeishuEmail != nil {
		user.FeishuEmail = strings.ToLower(strings.TrimSpace(*req.FeishuEmail))
	}
	if req.FeishuOpenID != nil {
		user.FeishuOpenID = *req.FeishuOpenID
	}
	if req.FeishuUnionID != nil {
		user.FeishuUnionID = *req.FeishuUnionID
	}
	if req.FeishuChatID != nil {
		user.FeishuChatID = *req.FeishuChatID
	}
	if req.Active != nil {
		if user.Active != *req.Active {
			shouldMarkSync = true
		}
		user.Active = *req.Active
	}
	if req.ExpiresAt != nil {
		expiresAt, err := parseOptionalTime(req.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("解析过期时间失败: %w", err)
		}
		if (user.ExpiresAt == nil) != (expiresAt == nil) || (user.ExpiresAt != nil && expiresAt != nil && !user.ExpiresAt.Equal(*expiresAt)) {
			shouldMarkSync = true
			user.ExpiredSweptAt = nil // 过期时间变更（续期）后允许调度器重新处理
		}
		user.ExpiresAt = expiresAt
	}
	if user.FeishuOpenID != "" || user.FeishuUnionID != "" || user.FeishuChatID != "" {
		now := time.Now()
		user.FeishuBoundAt = &now
	} else {
		user.FeishuBoundAt = nil
	}
	if err := repository.DB.Transaction(func(tx *gorm.DB) error {
		if req.GroupIDs != nil {
			groupIDs, err := parseOptionalUintSlice(req.GroupIDs)
			if err != nil {
				return fmt.Errorf("解析分组失败: %w", err)
			}
			if err := s.userRepo.ReplaceGroupsTx(tx, user, groupIDs); err != nil {
				return fmt.Errorf("更新用户分组失败: %w", err)
			}
		}
		if err := tx.Save(user).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	updated, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("读取用户失败: %w", err)
	}
	if shouldMarkSync {
		if liveOn {
			s.applyUpdateLive(updated, oldNodeIDs, oldEffectiveActive, oldUsername)
		} else {
			_ = s.nodeRepo.MarkAllDrifted()
		}
	}
	return s.toResponse(updated, baseURL), nil
}

func (s *UserService) Delete(id uint) error {
	// live-apply 需在删除前捕获用户的节点集与 email（删除后查不到分组关联）
	var (
		liveOn  = s.liveSync.Enabled()
		nodeIDs []uint
		email   string
	)
	if liveOn {
		if user, err := s.userRepo.FindByID(id); err == nil {
			nodeIDs = s.userNodeIDs(user)
			email = user.Username
		} else {
			liveOn = false // 查不到用户，回退全量标记漂移
		}
	}

	if err := s.userRepo.Delete(id); err != nil {
		return err
	}

	if liveOn {
		s.liveSync.RemoveUserLive(nodeIDs, email)
		return nil
	}
	return s.nodeRepo.MarkAllDrifted()
}

func (s *UserService) ToggleActive(id uint) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return errors.New("用户不存在")
	}
	newActive := !user.Active
	if err := s.userRepo.UpdateActive(id, newActive); err != nil {
		return err
	}
	// live-apply：启用 → AddUser（已过期用户不加，配置生成也不含它），禁用 → RemoveUser
	if s.liveSync.Enabled() {
		if newActive {
			if !user.IsExpired(time.Now()) {
				s.liveSync.AddUserLive(s.userNodeIDs(user), user.Username, user.UUID)
			}
		} else {
			s.liveSync.RemoveUserLive(s.userNodeIDs(user), user.Username)
		}
		return nil
	}
	return s.nodeRepo.MarkAllDrifted()
}

// SweepResult 一次过期摘除的汇总
type SweepResult struct {
	Swept     []string // 已处理的用户名
	LiveApply bool     // 本次是否走 gRPC 即时摘除（false = 标记漂移等待 restart 同步）
}

// SweepExpired 把已过期但仍启用的用户从节点运行时摘除（给调度器周期调用）。
//   - live-apply on：逐用户 RemoveUserLive（失败节点已在 livesync 内标记漂移兜底）
//   - live-apply off：MarkAllDrifted 一次，交给 restart 同步路径
//
// 处理过的用户打 ExpiredSweptAt 标记，不重复扫；续期会清标记。
// 订阅层的 403 拦截与本方法互补：前者挡新取订阅，后者断已连会话。
func (s *UserService) SweepExpired(now time.Time) (*SweepResult, error) {
	users, err := s.userRepo.FindExpiredUnswept(now)
	if err != nil {
		return nil, fmt.Errorf("查询过期用户失败: %w", err)
	}
	result := &SweepResult{LiveApply: s.liveSync.Enabled()}
	if len(users) == 0 {
		return result, nil
	}
	if !result.LiveApply {
		if err := s.nodeRepo.MarkAllDrifted(); err != nil {
			return nil, fmt.Errorf("标记节点漂移失败: %w", err)
		}
	}
	for i := range users {
		u := &users[i]
		if result.LiveApply {
			s.liveSync.RemoveUserLive(s.userNodeIDs(u), u.Username)
		}
		if err := s.userRepo.MarkExpiredSwept(u.ID, now); err != nil {
			return result, fmt.Errorf("标记用户 %s 已摘除失败: %w", u.Username, err)
		}
		result.Swept = append(result.Swept, u.Username)
	}
	return result, nil
}

func (s *UserService) List(page, pageSize int, baseURL string) ([]dto.UserResponse, int64, error) {
	users, total, err := s.userRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	// 一次性 join 当页用户的累计流量，避免 N+1
	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	totals, _ := s.trafficRepo.ListTotalsByUserIDs(ids) // 失败时空 map，不阻塞主流程

	result := make([]dto.UserResponse, 0, len(users))
	for i := range users {
		resp := s.toResponse(&users[i], baseURL)
		if t, ok := totals[users[i].ID]; ok {
			resp.TrafficUpBytes = t.UpBytes
			resp.TrafficDownBytes = t.DownBytes
			if !t.LastUpdatedAt.IsZero() {
				resp.TrafficLastUpdatedAt = t.LastUpdatedAt.Format(time.RFC3339)
			}
		}
		result = append(result, *resp)
	}
	return result, total, nil
}

// ResetUUID 重置用户 UUID（触发全节点重新同步）
func (s *UserService) ResetUUID(id uint, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	newUUID := uuid.NewString()
	if err := s.userRepo.UpdateUUID(id, newUUID); err != nil {
		return nil, fmt.Errorf("重置 UUID 失败: %w", err)
	}
	user.UUID = newUUID
	if s.liveSync.Enabled() {
		// UUID 变更：email 不变，逐 inbound 先删旧账号再加新 UUID 账号。
		// 未激活/已过期用户不在任何节点运行时（config 已过滤），UUID 变更无 config 影响 → no-op。
		if user.EffectiveActive(time.Now()) {
			s.liveSync.ReplaceUserLive(s.userNodeIDs(user), user.Username, newUUID)
		}
	} else {
		_ = s.nodeRepo.MarkAllDrifted()
	}
	return s.toResponse(user, baseURL), nil
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
	return s.toResponse(user, baseURL), nil
}

func (s *UserService) toResponse(u *entity.User, baseURL string) *dto.UserResponse {
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
