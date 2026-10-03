package auth

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// SafeRequestLogger excludes all query strings and handler error bodies at the
// logger boundary, before credential-bearing callbacks can enter access logs.
func SafeRequestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		path := strings.SplitN(param.Path, "?", 2)[0]
		return fmt.Sprintf("[HTTP] status=%d method=%s path=%q\n", param.StatusCode, param.Method, path)
	})
}
