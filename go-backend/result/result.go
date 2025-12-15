package result

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Result struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Ts   int64       `json:"ts"`
	Data interface{} `json:"data"`
}

func NewResult() *Result {
	return &Result{
		Code: 0,
		Msg:  "操作成功",
		Ts:   time.Now().UnixMilli(),
	}
}

func Ok(data interface{}) *Result {
	r := NewResult()
	r.Data = data
	return r
}

func OkMsg(msg string) *Result {
	r := NewResult()
	r.Msg = msg
	return r
}

func Err(code int, msg string) *Result {
	return &Result{
		Code: code,
		Msg:  msg,
		Ts:   time.Now().UnixMilli(),
	}
}

func Fail(msg string) *Result {
	return Err(-1, msg)
}

// Gin 辅助函数
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Ok(data))
}

func Error(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Err(code, msg))
}
