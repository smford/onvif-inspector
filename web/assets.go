package web

import "embed"

// Assets embeds the web dashboard static frontend files
//
//go:embed index.html style.css app.js
var Assets embed.FS
