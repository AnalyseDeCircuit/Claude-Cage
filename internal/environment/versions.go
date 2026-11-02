package environment

import (
	"context"
	"encoding/json"

	"github.com/sylvinus/agent-vm/internal/cage"
)

type ComponentVersions struct {
	Status         string              `json:"status"`
	UpdateRequired bool                `json:"update_required"`
	Required       cage.GuestManifest  `json:"required"`
	Current        *cage.GuestManifest `json:"current"`
}
type ToolVersion struct {
	Current  string `json:"current"`
	Required string `json:"required"`
}

type Versions struct {
	Running    bool
	Components ComponentVersions
	Tools      map[string]ToolVersion
	Error      string
}

// Versions runs tool probes only on demand, not during connection/status polling.
func (r *Runtime) Versions(ctx context.Context) Versions {
	running := r.Running()
	components := ComponentVersions{Status: "stopped", Required: cage.RequiredGuestManifest()}
	tools := map[string]ToolVersion{}
	for name, required := range cage.RequiredToolVersions() {
		tools[name] = ToolVersion{Required: required}
	}
	var detail string
	if running {
		data, err := r.Guest(ctx, `python3 - <<'CAGE_VERSIONS'
import json,subprocess,os
from pathlib import Path
p=Path('/etc/claude-cage/components.json')
try: manifest=json.loads(p.read_text())
except (OSError,ValueError): manifest=None
versions={}
env={**os.environ,'DISABLE_AUTOUPDATER':'1','PATH':str(Path.home()/'.local/bin')+':'+os.environ.get('PATH','')}
for name,args in {'chromium':['chromium','--version'],'code_server':['code-server','--version'],'claude':['claude','--version'],'mise':[str(Path.home()/'.local/bin/mise'),'--version']}.items():
 try:
  result=subprocess.run(args,env=env,capture_output=True,text=True,timeout=4)
  versions[name]=result.stdout.strip().splitlines()[0][:200] if result.returncode==0 and result.stdout.strip() else ''
 except (OSError,subprocess.TimeoutExpired): versions[name]=''
print(json.dumps({'manifest':manifest,'tools':versions}))
CAGE_VERSIONS`)
		var snapshot struct {
			Manifest *cage.GuestManifest `json:"manifest"`
			Tools    map[string]string   `json:"tools"`
		}
		if err != nil || json.Unmarshal(data, &snapshot) != nil {
			components.Status = "unknown"
			detail = "环境版本暂时无法读取，请恢复连接后重试。"
		} else {
			components.Current = snapshot.Manifest
			components.UpdateRequired = snapshot.Manifest == nil || !snapshot.Manifest.Current()
			components.Status = "current"
			if components.UpdateRequired {
				components.Status = "update_required"
			}
			for name, current := range snapshot.Tools {
				entry := tools[name]
				entry.Current = current
				tools[name] = entry
			}
		}
	}

	return Versions{running, components, tools, detail}
}
