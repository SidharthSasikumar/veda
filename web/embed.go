package web

import "embed"

//go:embed index.html app.js style.css lab.html hub.js hub.css agents.js agents.css agent-model.js office.js office.css office-model.js changes.js changes.css changes-model.js vendor/*
var Files embed.FS
