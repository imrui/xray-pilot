package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

type FeishuHandler struct {
	svc *service.FeishuService
}

var feishuEventDeduper = struct {
	mu   sync.Mutex
	seen map[string]time.Time
}{
	seen: make(map[string]time.Time),
}

func NewFeishuHandler() *FeishuHandler {
	return &FeishuHandler{svc: service.NewFeishuService()}
}

type feishuChallengeRequest struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
}

type feishuEncryptedRequest struct {
	Encrypt string `json:"encrypt"`
}

type feishuEventRequest struct {
	Header struct {
		EventType string `json:"event_type"`
		Token     string `json:"token"`
		EventID   string `json:"event_id"`
	} `json:"header"`
	Event struct {
		Sender struct {
			SenderID struct {
				OpenID  string `json:"open_id"`
				UnionID string `json:"union_id"`
			} `json:"sender_id"`
		} `json:"sender"`
		Message struct {
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
			ChatType    string `json:"chat_type"`
		} `json:"message"`
	} `json:"event"`
}

func (h *FeishuHandler) Events(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "invalid request body"})
		return
	}

	var encrypted feishuEncryptedRequest
	if err := json.Unmarshal(raw, &encrypted); err == nil && strings.TrimSpace(encrypted.Encrypt) != "" {
		plain, err := h.svc.DecryptEvent(strings.TrimSpace(encrypted.Encrypt))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"msg": err.Error()})
			return
		}
		raw = plain
	}

	var challenge feishuChallengeRequest
	if err := json.Unmarshal(raw, &challenge); err == nil && challenge.Challenge != "" {
		if !h.svc.ValidateVerificationToken(challenge.Token) {
			c.JSON(http.StatusUnauthorized, gin.H{"msg": "invalid verification token"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"challenge": challenge.Challenge})
		return
	}

	var req feishuEventRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
		return
	}

	if !h.svc.IsEnabled() {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "feishu disabled"})
		return
	}

	token := strings.TrimSpace(req.Header.Token)
	if !h.svc.ValidateVerificationToken(token) {
		c.JSON(http.StatusUnauthorized, gin.H{"msg": "invalid verification token"})
		return
	}

	if req.Header.EventType != "im.message.receive_v1" {
		c.JSON(http.StatusOK, gin.H{"code": 0})
		return
	}
	if req.Event.Message.ChatType != "" && req.Event.Message.ChatType != "p2p" {
		c.JSON(http.StatusOK, gin.H{"code": 0})
		return
	}
	if req.Event.Message.MessageType != "text" {
		c.JSON(http.StatusOK, gin.H{"code": 0})
		return
	}

	openID := strings.TrimSpace(req.Event.Sender.SenderID.OpenID)
	unionID := strings.TrimSpace(req.Event.Sender.SenderID.UnionID)
	if openID == "" && unionID == "" {
		c.JSON(http.StatusOK, gin.H{"code": 0})
		return
	}

	eventID := strings.TrimSpace(req.Header.EventID)
	if eventID != "" && isDuplicateFeishuEvent(eventID) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "duplicate ignored"})
		return
	}

	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(req.Event.Message.Content), &content); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0})
		return
	}

	keyword := strings.TrimSpace(strings.ToLower(content.Text))
	h.svc.RecordWebhookDebug(openID, unionID, keyword)
	c.JSON(http.StatusOK, gin.H{"code": 0})

	go func(openID, unionID, keyword string) {
		switch keyword {
		case "帮助", "help":
			_ = h.svc.SendHelpMessage(openID)
		case "订阅", "链接", "二维码":
			user, err := h.svc.ResolveAuthorizedUser(openID, unionID)
			if err != nil || user == nil {
				_ = h.svc.SendUnboundMessage(openID)
				return
			}
			if !user.FeishuEnabled {
				_ = h.svc.SendTextMessageForUser(openID, h.svc.BuildDisabledText())
				return
			}
			_ = h.svc.SendSubscriptionMessage(openID, user)
		default:
			_ = h.svc.SendHelpMessage(openID)
		}
	}(openID, unionID, keyword)
}

func isDuplicateFeishuEvent(eventID string) bool {
	now := time.Now()
	feishuEventDeduper.mu.Lock()
	defer feishuEventDeduper.mu.Unlock()

	for id, ts := range feishuEventDeduper.seen {
		if now.Sub(ts) > 10*time.Minute {
			delete(feishuEventDeduper.seen, id)
		}
	}
	if _, exists := feishuEventDeduper.seen[eventID]; exists {
		return true
	}
	feishuEventDeduper.seen[eventID] = now
	return false
}

func (h *FeishuHandler) PushUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	result, err := h.svc.PushSubscriptionToUserID(uint(id))
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *FeishuHandler) BindUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.svc.BindUserByEmail(uint(id), req.Email)
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{
		"id":                    user.ID,
		"feishu_enabled":        user.FeishuEnabled,
		"feishu_email":          user.FeishuEmail,
		"feishu_open_id":        user.FeishuOpenID,
		"feishu_union_id":       user.FeishuUnionID,
		"feishu_chat_id":        user.FeishuChatID,
		"feishu_identity_ready": user.FeishuOpenID != "" || user.FeishuChatID != "",
		"feishu_bound_at": func() string {
			if user.FeishuBoundAt == nil {
				return ""
			}
			return user.FeishuBoundAt.Format(time.RFC3339)
		}(),
	})
}

func (h *FeishuHandler) UnbindUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}

	user, err := h.svc.UnbindUser(uint(id))
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{
		"id":                    user.ID,
		"feishu_enabled":        user.FeishuEnabled,
		"feishu_email":          user.FeishuEmail,
		"feishu_open_id":        user.FeishuOpenID,
		"feishu_union_id":       user.FeishuUnionID,
		"feishu_chat_id":        user.FeishuChatID,
		"feishu_identity_ready": false,
		"feishu_bound_at":       "",
	})
}

func (h *FeishuHandler) PushUsers(c *gin.Context) {
	var req struct {
		UserIDs []uint `json:"user_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.PushSubscriptionToUsers(req.UserIDs)
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, result)
}
