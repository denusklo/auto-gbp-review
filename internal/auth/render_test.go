package auth

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	SetRenderer(func(c *gin.Context, layout, content string, data gin.H) {
		tmpl, err := template.ParseFiles(filepath.Join("../..", layout), filepath.Join("../..", content))
		if err != nil {
			c.String(http.StatusInternalServerError, "Template unavailable")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(c.Writer, data); err != nil {
			c.Status(http.StatusInternalServerError)
		}
	})
	os.Exit(m.Run())
}
