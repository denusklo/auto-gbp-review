package auth

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetRenderer installs the application's template renderer before serving requests.
func SetRenderer(renderer func(*gin.Context, string, string, gin.H)) { renderPage = renderer }

var renderPage = func(c *gin.Context, layout, content string, data gin.H) {
	tmpl, err := template.ParseFiles(layout, content)
	if err != nil {
		c.String(http.StatusInternalServerError, "Unable to render authentication page")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}
