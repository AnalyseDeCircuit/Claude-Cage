package cage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"al.essio.dev/pkg/shellescape"
	"github.com/sylvinus/agent-vm/internal/version"
)

// Script builders below own guest installation and startup order. The embedded
// files are declared in guest_resources.go; guest/ owns their runtime code.
func browserServices() string {
	return guestServices + "\nsudo tee /usr/local/lib/browser-downloads.mjs >/dev/null <<'CAGE_BROWSER_DOWNLOADS'\n" + browserDownloads + "\nCAGE_BROWSER_DOWNLOADS\n" + "\nsudo tee /usr/local/bin/cage-editor-migrate >/dev/null <<'CAGE_EDITOR_MIGRATE'\n" + editorMigrate + "\nCAGE_EDITOR_MIGRATE\nsudo chmod 0755 /usr/local/bin/cage-editor-migrate\n" + "\nsudo tee /usr/local/lib/browser-audit.mjs >/dev/null <<'CAGE_BROWSER_AUDIT'\n" + browserAudit + "\nCAGE_BROWSER_AUDIT\n" + "\nsudo tee /usr/local/lib/cage-browser-agent.mjs >/dev/null <<'CAGE_BROWSER_AGENT'\n" + browserAgent + "\nCAGE_BROWSER_AGENT\n" + "sudo tee /usr/local/bin/cage-save-download >/dev/null <<'CAGE_DOWNLOAD_HELPER'\n" + saveDownload + "\nCAGE_DOWNLOAD_HELPER\nsudo chmod 0755 /usr/local/bin/cage-save-download\nprintf 'cage-browser ALL=(root) NOPASSWD: /usr/local/bin/cage-save-download *\\n' | sudo tee /etc/sudoers.d/cage-download >/dev/null\nsudo chmod 0440 /etc/sudoers.d/cage-download\n" + "\nsudo python3 - <<'CAGE_NOVNC_PATCH'\n" + novncPatch + "\nCAGE_NOVNC_PATCH\n" + "\nsudo tee /usr/share/novnc/cage-clipboard.js >/dev/null <<'CAGE_CLIPBOARD_JS'\n" + clipboardJS + "\nCAGE_CLIPBOARD_JS\n" + `sudo python3 - <<'PYCODE'
from pathlib import Path
import re
p = Path("/usr/share/novnc/vnc.html")
s = p.read_text()
tag = '<script type="module" src="cage-clipboard.js?cage=8"></script>'
pattern = r'<script type="module" src="cage-clipboard\.js(?:\?cage=\d+)?"></script>'
if re.search(pattern, s):
    s = re.sub(pattern, lambda _: tag, s)
else:
    s = s.replace("</body>", tag + "\n</body>")
p.write_text(s)
PYCODE
`
}

var GuestSetup = guestDependencies + "\n" + browserServices()

func ToolInstallPrelude() string { return toolVersions + "\n" + guestTools + "\n" }

func RequiredToolVersions() map[string]string {
	result := map[string]string{"chromium": "Debian security repository"}
	for _, line := range strings.Split(toolVersions, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "CAGE_CODE_SERVER_VERSION":
			result["code_server"] = value
		case "CAGE_MISE_VERSION":
			result["mise"] = value
		case "CAGE_CLAUDE_VERSION":
			result["claude"] = value
		}
	}
	return result
}

func ToolsUpdateScript() string {
	return "set -euo pipefail\nexport PATH=\"$HOME/.local/bin:$PATH\"\n" + ToolInstallPrelude() + `
sudo env DEBIAN_FRONTEND=noninteractive dpkg --configure -a </dev/null
sudo env DEBIAN_FRONTEND=noninteractive apt-get update </dev/null
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --only-upgrade chromium </dev/null
cage_install_tool mise
cage_install_tool code-server
cage_install_tool claude
printf 'Cage tool packages are ready; applying browser and editor components.\n'
`
}

func (c Config) EnvironmentScript() string {
	return fmt.Sprintf("export CAGE_TZ=%s\nexport CAGE_LOCALE=%s\nexport CAGE_LANGUAGE=%s\n", shellescape.Quote(c.Timezone), shellescape.Quote(c.Locale), shellescape.Quote(c.BrowserLanguage)) + guestEnv
}

func (c Config) StartScript() string {
	body := c.EnvironmentScript() + "\n" + componentStartScript()
	return markGuestStart(body)
}

func componentStartScript() string {
	return browserServices() + "\nsudo tee /etc/claude-cage/editor-theme.json >/dev/null <<'CAGE_EDITOR_THEME'\n" + editorTheme + "\nCAGE_EDITOR_THEME\nif systemctl is-active --quiet cage-editor.service; then sudo systemctl stop cage-editor.service; fi\n" + guestStart
}

func markGuestStart(body string) string {
	manifest, _ := json.Marshal(RequiredGuestManifest())
	return "set -euo pipefail\nsudo rm -f /etc/claude-cage/components.json\n(\n" + body + "\n)\nsudo python3 - <<'CAGE_COMPONENT_MANIFEST'\nfrom pathlib import Path\nimport os,tempfile\np=Path('/etc/claude-cage/components.json')\nfd,name=tempfile.mkstemp(prefix='.components-',dir=p.parent)\nwith os.fdopen(fd,'w') as f:\n f.write(" + fmt.Sprintf("%q", string(manifest)) + ")\n f.flush();os.fsync(f.fileno())\nos.chmod(name,0o644)\nos.replace(name,p)\nCAGE_COMPONENT_MANIFEST\n"
}

type GuestManifest struct {
	Protocol   int    `json:"protocol"`
	Revision   string `json:"revision"`
	AppVersion string `json:"app_version"`
}

var guestRevision = sync.OnceValue(func() string {
	payload := strings.Join([]string{"cage-guest-protocol-1", guestDependencies, guestEnv, componentStartScript(), ToolInstallPrelude()}, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
})

func RequiredGuestManifest() GuestManifest {
	return GuestManifest{Protocol: 1, Revision: guestRevision(), AppVersion: version.Version}
}

func (m GuestManifest) Current() bool {
	required := RequiredGuestManifest()
	return m.Protocol == required.Protocol && m.Revision == required.Revision
}
