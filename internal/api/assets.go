package api

import _ "embed"

// The official self-hosted Supermemory welcome page and its Memory-tab bundle,
// extracted from supermemory-server v0.0.8 (embedded there via go:embed).
// Serving these verbatim is what makes this daemon look like the official
// local console to a browser.

//go:embed assets/index.html
var welcomePage []byte

//go:embed assets/local-console.js
var localConsoleJS []byte
