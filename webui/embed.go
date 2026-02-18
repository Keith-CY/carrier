package webui

import "embed"

//go:embed static/*
var staticFiles embed.FS
