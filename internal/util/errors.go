package util

import (
	"github.com/gin-gonic/gin"
)

type ErrorResp struct {
	ErrorCode string                 `json:"error_code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

func Error(c *gin.Context, status int, code string, msg string, details map[string]interface{}) {
	c.JSON(status, ErrorResp{ErrorCode: code, Message: msg, Details: details})
}
