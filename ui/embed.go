// Package ui embeds the dashboard assets so the proxy can always serve
// them, even when launched from a directory that has no ui/ folder.
package ui

import "embed"

//go:embed index.html styles.css app.js
var Assets embed.FS