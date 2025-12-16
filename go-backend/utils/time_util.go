package utils

import "time"

// CurrentTimeMillis 返回当前时间的毫秒时间戳
func CurrentTimeMillis() int64 {
	return time.Now().UnixMilli()
}
