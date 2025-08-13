//go:build withui
package main

import (
	"io/fs"
	"embed"
	//"net/http"
)

// content holds our static web server content.
//go:embed webui/*
var embeddedUI embed.FS
var subFS fs.FS

func InitEmbeddedFs() {
	subFS, _ = fs.Sub(embeddedUI, "webui")
}