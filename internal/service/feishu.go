package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type FeishuService struct {
	settingSvc *SettingService
	userRepo   *repository.UserRepository
	logRepo    *repository.LogRepository
	httpClient *http.Client
}

func NewFeishuService() *FeishuService {
	return &FeishuService{
		settingSvc: NewSettingService(),
		userRepo:   repository.NewUserRepository(),
		logRepo:    repository.NewLogRepository(),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *FeishuService) GetStatus() *dto.FeishuStatusResponse {
	enabled := strings.EqualFold(strings.TrimSpace(s.settingSvc.Get(KeyFeishuEnabled)), "true")
	appID := strings.TrimSpace(s.settingSvc.Get(KeyFeishuAppID))
	appSecret := strings.TrimSpace(s.settingSvc.Get(KeyFeishuAppSecret))
	verificationToken := strings.TrimSpace(s.settingSvc.Get(KeyFeishuVerificationToken))
	baseURL := strings.TrimRight(strings.TrimSpace(s.settingSvc.Get(KeyFeishuBaseURL)), "/")
	botName := strings.TrimSpace(s.settingSvc.Get(KeyFeishuBotName))

	missing := make([]string, 0, 4)
	if appID == "" {
		missing = append(missing, KeyFeishuAppID)
	}
	if appSecret == "" {
		missing = append(missing, KeyFeishuAppSecret)
	}
	if verificationToken == "" {
		missing = append(missing, KeyFeishuVerificationToken)
	}
	if baseURL == "" {
		missing = append(missing, KeyFeishuBaseURL)
	}

	status := &dto.FeishuStatusResponse{
		Enabled:     enabled,
		Configured:  len(missing) == 0,
		MissingKeys: missing,
		BotName:     botName,
	}
	if baseURL != "" {
		status.WebhookURL = baseURL + "/api/feishu/events"
	}
	return status
}

func (s *FeishuService) ValidateConfig() *dto.FeishuStatusResponse {
	return s.GetStatus()
}

func (s *FeishuService) IsEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(s.settingSvc.Get(KeyFeishuEnabled)), "true")
}

func (s *FeishuService) ValidateVerificationToken(token string) bool {
	expected := strings.TrimSpace(s.settingSvc.Get(KeyFeishuVerificationToken))
	return expected == "" || token == expected
}

func (s *FeishuService) DecryptEvent(encrypt string) ([]byte, error) {
	encryptKey := strings.TrimSpace(s.settingSvc.Get(KeyFeishuEncryptKey))
	if encryptKey == "" {
		return nil, fmt.Errorf("飞书 Encrypt Key 未配置")
	}

	raw, err := base64.StdEncoding.DecodeString(encrypt)
	if err != nil {
		return nil, fmt.Errorf("解码飞书加密事件失败: %w", err)
	}
	if len(raw) < aes.BlockSize || len(raw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("飞书加密事件长度无效")
	}

	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("初始化飞书事件解密失败: %w", err)
	}

	iv := raw[:aes.BlockSize]
	payload := make([]byte, len(raw)-aes.BlockSize)
	copy(payload, raw[aes.BlockSize:])
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(payload, payload)

	plain, err := pkcs7Unpad(payload, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("飞书事件去填充失败: %w", err)
	}
	return plain, nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid padding size")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, fmt.Errorf("invalid padding bytes")
		}
	}
	return data[:len(data)-padding], nil
}

func maskID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func maskEmail(value string) string {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, "@")
	if len(parts) != 2 || len(parts[0]) <= 2 {
		return value
	}
	return parts[0][:2] + "***@" + parts[1]
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *FeishuService) FindBoundUser(openID, unionID string) (*entity.User, error) {
	return s.userRepo.FindByFeishuIdentity(openID, unionID)
}

func (s *FeishuService) BuildHelpText() string {
	return "支持的指令：订阅、链接、二维码、帮助。发送“订阅”可获取个人订阅信息，发送“二维码”可打开订阅页查看二维码。"
}

func (s *FeishuService) RecordWebhookDebug(openID, unionID, keyword string) {
	s.logRepo.Record("feishu_webhook", "message", true, fmt.Sprintf("收到飞书消息，keyword=%s open_id=%s union_id=%s", keyword, maskID(openID), maskID(unionID)), 0)
}

func (s *FeishuService) BuildUnboundText() string {
	return "当前飞书账号尚未绑定订阅用户，请联系管理员。"
}

func (s *FeishuService) BuildDisabledText() string {
	return "当前账号尚未启用飞书消息功能，请联系管理员。"
}

func (s *FeishuService) BuildPendingBindText() string {
	return "当前飞书账号尚未完成身份绑定，请联系管理员确认飞书邮箱配置。"
}

func (s *FeishuService) BuildSubscriptionLinks(user *entity.User) (pageURL string, subURL string) {
	base := strings.TrimRight(strings.TrimSpace(s.settingSvc.Get(KeySubscriptionBaseURL)), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(s.settingSvc.Get(KeyFeishuBaseURL)), "/")
	}
	if base == "" {
		return "", ""
	}
	pageURL = fmt.Sprintf("%s/sub/%s", base, user.Token)
	subURL = pageURL + "?sub=1"
	return pageURL, subURL
}

func (s *FeishuService) SendHelpMessage(openID string) error {
	return s.sendTextMessage(openID, s.BuildHelpText())
}

func (s *FeishuService) SendUnboundMessage(openID string) error {
	return s.sendTextMessage(openID, s.BuildUnboundText())
}

func (s *FeishuService) SendTextMessageForUser(openID, text string) error {
	return s.sendTextMessage(openID, text)
}

func (s *FeishuService) SendSubscriptionMessage(openID string, user *entity.User) error {
	return s.sendSubscriptionContent(openID, "open_id", user)
}

func (s *FeishuService) SendSubscriptionToUser(user *entity.User) error {
	receiveID, receiveType, ok := s.resolveRecipient(user)
	if !ok {
		return fmt.Errorf("用户 %s 未绑定可用的飞书标识", user.Username)
	}
	return s.sendSubscriptionContent(receiveID, receiveType, user)
}

type FeishuUserProfile struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	EnterpriseEmail string `json:"enterprise_email"`
	OpenID          string `json:"open_id"`
	UnionID         string `json:"union_id"`
}

func (s *FeishuService) sendTextMessage(openID, text string) error {
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	return s.sendMessage(openID, "open_id", "text", string(content))
}

func (s *FeishuService) sendMessage(receiveID, receiveIDType, msgType, content string) error {
	token, err := s.getTenantAccessToken()
	if err != nil {
		return err
	}

	body, err := json.Marshal(map[string]string{
		"receive_id": receiveID,
		"msg_type":   msgType,
		"content":    content,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type="+receiveIDType,
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用飞书发送消息失败: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&payload); decodeErr != nil {
		return fmt.Errorf("解析飞书响应失败: %w", decodeErr)
	}
	if resp.StatusCode >= 300 || payload.Code != 0 {
		return fmt.Errorf("飞书发送失败: %s", payload.Msg)
	}
	return nil
}

func (s *FeishuService) sendSubscriptionContent(receiveID, receiveIDType string, user *entity.User) error {
	pageURL, subURL := s.BuildSubscriptionLinks(user)
	if pageURL == "" {
		return fmt.Errorf("subscription.base_url 未配置，无法生成飞书订阅链接")
	}

	statusText := "正常"
	if !user.Active {
		statusText = "已禁用"
	}
	expireText := "永久有效"
	if user.ExpiresAt != nil {
		expireText = user.ExpiresAt.Format("2006-01-02 15:04")
	}

	cardContent := map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
			"enable_forward":   true,
		},
		"header": map[string]any{
			"template": "green",
			"title": map[string]string{
				"tag":     "plain_text",
				"content": "Xray Pilot 订阅信息",
			},
		},
		"elements": []map[string]any{
			{
				"tag": "div",
				"text": map[string]string{
					"tag": "lark_md",
					"content": fmt.Sprintf(
						"**用户名**：%s\n**状态**：%s\n**到期时间**：%s",
						user.Username,
						statusText,
						expireText,
					),
				},
			},
			{
				"tag": "action",
				"actions": []map[string]any{
					{
						"tag":  "button",
						"type": "primary",
						"text": map[string]string{
							"tag":     "plain_text",
							"content": "打开订阅页",
						},
						"url": pageURL,
					},
				},
			},
			{
				"tag": "div",
				"text": map[string]string{
					"tag": "lark_md",
					"content": "**客户端订阅链接**\n导入 v2rayN、Clash 等客户端时，请复制下方链接；如需二维码或节点详情，请打开订阅页。",
				},
			},
			{
				"tag": "div",
				"text": map[string]string{
					"tag":     "plain_text",
					"content": subURL,
				},
			},
		},
	}

	content, err := json.Marshal(cardContent)
	if err != nil {
		return err
	}
	return s.sendMessage(receiveID, receiveIDType, "interactive", string(content))
}

func (s *FeishuService) resolveRecipient(user *entity.User) (receiveID string, receiveType string, ok bool) {
	if !user.FeishuEnabled {
		return "", "", false
	}
	if strings.TrimSpace(user.FeishuOpenID) != "" {
		return strings.TrimSpace(user.FeishuOpenID), "open_id", true
	}
	if strings.TrimSpace(user.FeishuChatID) != "" {
		return strings.TrimSpace(user.FeishuChatID), "chat_id", true
	}
	return "", "", false
}

type feishuEmailLookupResult struct {
	Email   string
	OpenID  string
	UnionID string
}

func (s *FeishuService) lookupUserByEmail(email string) (*feishuEmailLookupResult, error) {
	token, err := s.getTenantAccessToken()
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(map[string]any{
		"emails":           []string{strings.TrimSpace(email)},
		"include_resigned": false,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://open.feishu.cn/open-apis/contact/v3/users/batch_get_id?user_id_type=open_id",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("通过飞书邮箱查询用户失败: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserList []struct {
				Email   string `json:"email"`
				OpenID  string `json:"open_id"`
				UnionID string `json:"union_id"`
			} `json:"user_list"`
		} `json:"data"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&payload); decodeErr != nil {
		return nil, fmt.Errorf("解析飞书邮箱查询响应失败: %w", decodeErr)
	}
	if resp.StatusCode >= 300 || payload.Code != 0 {
		return nil, fmt.Errorf("通过飞书邮箱查询用户失败: %s", payload.Msg)
	}
	if len(payload.Data.UserList) == 0 {
		return nil, fmt.Errorf("未找到匹配的飞书用户")
	}
	item := payload.Data.UserList[0]
	emailMatched := strings.TrimSpace(item.Email)
	if emailMatched == "" {
		emailMatched = strings.TrimSpace(email)
	}
	return &feishuEmailLookupResult{
		Email:   emailMatched,
		OpenID:  strings.TrimSpace(item.OpenID),
		UnionID: strings.TrimSpace(item.UnionID),
	}, nil
}

func (s *FeishuService) BindUserByEmail(userID uint, email string) (*entity.User, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("飞书集成当前未启用")
	}
	status := s.GetStatus()
	if !status.Configured {
		return nil, fmt.Errorf("飞书配置不完整：%s", strings.Join(status.MissingKeys, "、"))
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}

	email = normalizeEmail(email)
	if email == "" {
		return nil, fmt.Errorf("请先填写飞书邮箱")
	}

	s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, "开始按邮箱绑定飞书："+maskEmail(email), 0)

	lookup, err := s.lookupUserByEmail(email)
	if err != nil {
		s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, err.Error(), 0)
		return nil, err
	}

	now := time.Now()
	previousEmail := normalizeEmail(user.FeishuEmail)
	normalizedEmail := normalizeEmail(lookup.Email)
	user.FeishuEnabled = true
	user.FeishuEmail = normalizedEmail
	if lookup.OpenID != "" {
		profile, err := s.GetUserProfile(lookup.OpenID)
		if err != nil {
			s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, err.Error(), 0)
			return nil, err
		}
		user.FeishuOpenID = strings.TrimSpace(profile.OpenID)
		user.FeishuUnionID = strings.TrimSpace(profile.UnionID)
		user.FeishuBoundAt = &now
	} else if previousEmail == "" || !strings.EqualFold(previousEmail, normalizedEmail) {
		user.FeishuOpenID = ""
		user.FeishuUnionID = ""
		user.FeishuBoundAt = nil
	}
	if err := s.userRepo.Update(user); err != nil {
		s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, err.Error(), 0)
		return nil, err
	}

	if user.FeishuOpenID != "" {
		s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, fmt.Sprintf("飞书绑定成功，email=%s open_id=%s", maskEmail(user.FeishuEmail), maskID(user.FeishuOpenID)), 0)
	} else {
		s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, fmt.Sprintf("飞书邮箱校验成功，等待用户首次私聊完成身份绑定，email=%s", maskEmail(user.FeishuEmail)), 0)
	}
	return user, nil
}

func (s *FeishuService) UnbindUser(userID uint) (*entity.User, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}

	user.FeishuOpenID = ""
	user.FeishuUnionID = ""
	user.FeishuChatID = ""
	user.FeishuBoundAt = nil
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	s.logRepo.Record("feishu_unbind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, "清除飞书绑定信息", 0)
	return user, nil
}

func (s *FeishuService) PushSubscriptionToUserID(userID uint) (*dto.FeishuPushResponse, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("飞书集成当前未启用")
	}
	status := s.GetStatus()
	if !status.Configured {
		return nil, fmt.Errorf("飞书配置不完整：%s", strings.Join(status.MissingKeys, "、"))
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}

	result := &dto.FeishuPushResponse{Total: 1}
	receiveID, receiveType, ok := s.resolveRecipient(user)
	if !ok {
		result.Skipped = 1
		result.Errors = []string{fmt.Sprintf("用户 %s 未绑定可用的飞书标识", user.Username)}
		return result, nil
	}

	if err := s.sendSubscriptionContent(receiveID, receiveType, user); err != nil {
		result.Failed = 1
		result.Errors = []string{fmt.Sprintf("用户 %s 推送失败：%v", user.Username, err)}
		s.logRepo.Record("feishu_push", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, err.Error(), 0)
		return result, nil
	}

	result.Sent = 1
	s.logRepo.Record("feishu_push", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, "飞书订阅推送成功", 0)
	return result, nil
}

func (s *FeishuService) PushSubscriptionToUsers(userIDs []uint) (*dto.FeishuPushResponse, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("飞书集成当前未启用")
	}
	status := s.GetStatus()
	if !status.Configured {
		return nil, fmt.Errorf("飞书配置不完整：%s", strings.Join(status.MissingKeys, "、"))
	}

	users, err := s.userRepo.FindByIDs(userIDs)
	if err != nil {
		return nil, err
	}

	result := &dto.FeishuPushResponse{Total: len(users)}
	for i := range users {
		user := &users[i]
		receiveID, receiveType, ok := s.resolveRecipient(user)
		if !ok {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("用户 %s 未绑定可用的飞书标识", user.Username))
			continue
		}
		if err := s.sendSubscriptionContent(receiveID, receiveType, user); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("用户 %s 推送失败：%v", user.Username, err))
			s.logRepo.Record("feishu_push", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, err.Error(), 0)
			continue
		}
		result.Sent++
		s.logRepo.Record("feishu_push", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, "飞书订阅推送成功", 0)
	}
	return result, nil
}

func (s *FeishuService) GetUserProfile(openID string) (*FeishuUserProfile, error) {
	token, err := s.getTenantAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodGet,
		"https://open.feishu.cn/open-apis/contact/v3/users/"+openID+"?user_id_type=open_id",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logRepo.Record("feishu_profile_fetch", "contact_api", false, "调用飞书用户资料接口失败："+err.Error(), 0)
		return nil, fmt.Errorf("获取飞书用户资料失败: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			User FeishuUserProfile `json:"user"`
		} `json:"data"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&payload); decodeErr != nil {
		s.logRepo.Record("feishu_profile_fetch", "contact_api", false, "解析飞书用户资料失败："+decodeErr.Error(), 0)
		return nil, fmt.Errorf("解析飞书用户资料失败: %w", decodeErr)
	}
	if resp.StatusCode >= 300 || payload.Code != 0 {
		s.logRepo.Record("feishu_profile_fetch", "contact_api", false, "飞书用户资料接口返回错误："+payload.Msg, 0)
		return nil, fmt.Errorf("获取飞书用户资料失败: %s", payload.Msg)
	}
	s.logRepo.Record("feishu_profile_fetch", "contact_api", true, fmt.Sprintf("成功获取飞书用户资料，open_id=%s email=%s enterprise_email=%s", maskID(openID), maskEmail(payload.Data.User.Email), maskEmail(payload.Data.User.EnterpriseEmail)), 0)
	return &payload.Data.User, nil
}

func (s *FeishuService) ResolveAuthorizedUser(openID, unionID string) (*entity.User, error) {
	user, err := s.FindBoundUser(openID, unionID)
	if err == nil && user != nil {
		return user, nil
	}

	profile, profileErr := s.GetUserProfile(openID)
	if profileErr != nil {
		s.logRepo.Record("feishu_bind", "webhook", false, "获取飞书资料失败："+profileErr.Error(), 0)
		return nil, profileErr
	}

	email := normalizeEmail(profile.Email)
	if email == "" {
		email = normalizeEmail(profile.EnterpriseEmail)
	}
	if email == "" {
		s.logRepo.Record("feishu_bind", "webhook", false, fmt.Sprintf("飞书资料未返回邮箱，open_id=%s", maskID(openID)), 0)
		return nil, fmt.Errorf("飞书资料中未返回邮箱")
	}
	s.logRepo.Record("feishu_bind", "webhook", true, fmt.Sprintf("首次私聊识别到飞书邮箱，email=%s open_id=%s", maskEmail(email), maskID(openID)), 0)

	user, err = s.userRepo.FindByFeishuEmail(email)
	if err != nil {
		s.logRepo.Record("feishu_bind", "webhook", false, fmt.Sprintf("按飞书邮箱匹配后台用户失败，email=%s err=%v", maskEmail(email), err), 0)
		return nil, err
	}
	if !user.FeishuEnabled {
		s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, fmt.Sprintf("飞书邮箱已匹配但用户未启用飞书消息，email=%s", maskEmail(email)), 0)
		return user, nil
	}

	changed := false
	if strings.TrimSpace(user.FeishuEmail) == "" {
		user.FeishuEmail = email
		changed = true
	}
	if strings.TrimSpace(user.FeishuOpenID) == "" && strings.TrimSpace(openID) != "" {
		user.FeishuOpenID = strings.TrimSpace(openID)
		changed = true
	}
	if strings.TrimSpace(user.FeishuUnionID) == "" && strings.TrimSpace(unionID) != "" {
		user.FeishuUnionID = strings.TrimSpace(unionID)
		changed = true
	}
	if user.FeishuBoundAt == nil && (user.FeishuOpenID != "" || user.FeishuUnionID != "") {
		now := time.Now()
		user.FeishuBoundAt = &now
		changed = true
	}
	if changed {
		if err := s.userRepo.Update(user); err != nil {
			s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), false, "首次私聊补全飞书身份失败："+err.Error(), 0)
			return nil, err
		}
		s.logRepo.Record("feishu_bind", fmt.Sprintf("user:%s(%d)", user.Username, user.ID), true, fmt.Sprintf("首次私聊已补全飞书身份，email=%s open_id=%s", maskEmail(email), maskID(user.FeishuOpenID)), 0)
	}
	return user, nil
}

func (s *FeishuService) getTenantAccessToken() (string, error) {
	appID := strings.TrimSpace(s.settingSvc.Get(KeyFeishuAppID))
	appSecret := strings.TrimSpace(s.settingSvc.Get(KeyFeishuAppSecret))
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("飞书 App ID / App Secret 未配置")
	}

	body, err := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取飞书 tenant_access_token 失败: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&payload); decodeErr != nil {
		return "", fmt.Errorf("解析飞书 token 响应失败: %w", decodeErr)
	}
	if resp.StatusCode >= 300 || payload.Code != 0 || payload.TenantAccessToken == "" {
		return "", fmt.Errorf("获取飞书 token 失败: %s", payload.Msg)
	}
	return payload.TenantAccessToken, nil
}
