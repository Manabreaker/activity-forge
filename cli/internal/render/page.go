package render

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"io"
)

//go:embed assets/index.html
var pageHTML string

//go:embed assets/style.css
var pageCSS string

//go:embed assets/app.js
var pageJS string

type bootstrap struct {
	View
	Offline bool   `json:"offline"`
	Token   string `json:"token"`
}

func Page(w io.Writer, v View, offline bool, token string) error {
	data, err := json.Marshal(bootstrap{View: v, Offline: offline, Token: token})
	if err != nil {
		return err
	}
	t, err := template.New("page").Parse(pageHTML)
	if err != nil {
		return err
	}
	return t.Execute(w, struct {
		Data  template.JS
		CSS   template.CSS
		JS    template.JS
		Nonce string
	}{
		Data: template.JS(data), CSS: template.CSS(pageCSS), JS: template.JS(pageJS), Nonce: token})
}
