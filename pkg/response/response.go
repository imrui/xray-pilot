package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/dto"
)

// Success 返回成功响应
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, dto.Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Fail 返回错误响应
func Fail(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, dto.Response{
		Code:    code,
		Message: message,
	})
}

// BadRequest 参数错误
func BadRequest(c *gin.Context, message string) {
	Fail(c, 400, message)
}

// Unauthorized 未授权
func Unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, dto.Response{
		Code:    401,
		Message: "未授权，请登录",
	})
}

// PageSuccess 返回分页成功响应
func PageSuccess(c *gin.Context, total int64, list any) {
	Success(c, dto.PageResult{
		Total: total,
		List:  list,
	})
}
