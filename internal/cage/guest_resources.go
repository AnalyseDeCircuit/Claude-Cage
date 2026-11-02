package cage

import _ "embed"

// Embed individual runtime resources so tests beside them are not shipped.
// Repository paths do not determine installation paths inside the guest.

//go:embed guest/guest-setup.sh
var guestDependencies string

//go:embed guest/guest-services.sh
var guestServices string

//go:embed guest/novnc-patch.py
var novncPatch string

//go:embed guest/clipboard.js
var clipboardJS string

//go:embed guest/browser-agent.mjs
var browserAgent string

//go:embed guest/browser-audit.mjs
var browserAudit string

//go:embed guest/browser-downloads.mjs
var browserDownloads string

//go:embed guest/editor-migrate.py
var editorMigrate string

//go:embed guest/save-download.py
var saveDownload string

//go:embed guest/editor-theme.json
var editorTheme string

//go:embed guest/guest-start.sh
var guestStart string

//go:embed guest/guest-env.sh
var guestEnv string

//go:embed guest/tool-versions.env
var toolVersions string

//go:embed guest/guest-tools.sh
var guestTools string
