package web

import "embed"

//go:embed index.html app.js style.css lab.html hub.js hub.css vendor/*
var Files embed.FS
