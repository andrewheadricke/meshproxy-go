//go:build withui

package main

import "embed"

// content holds our static web server content.
//go:embed webui/*
var content embed.FS