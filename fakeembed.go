//go:build !withui
package main

import (
	"io/fs"
)

var subFS fs.FS

func InitEmbeddedFs() {
	//subFS, _ = fs.Sub(embeddedUI, "webui")
}