# Publish browser assets while the web endpoint is stopped, so clients cannot
# mix old and new noVNC modules during an upgrade.
if systemctl is-active --quiet cage-webvnc; then
  sudo systemctl stop cage-webvnc
fi
if ! command -v X0tigervnc >/dev/null || ! command -v Xtigervnc >/dev/null || ! command -v openbox >/dev/null || ! command -v xrandr >/dev/null; then
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tigervnc-standalone-server tigervnc-scraping-server tigervnc-tools openbox x11-xserver-utils
fi
sudo install -d -m 0755 /etc/claude-cage /usr/local/lib
sudo tee /usr/local/bin/cage-open >/dev/null <<'SCRIPT'
#!/bin/sh
set -eu
case "${1:-}" in http://*|https://*|about:blank) ;; *) echo 'Only HTTP(S) browser URLs are accepted.' >&2; exit 2;; esac
exec sudo -u cage-browser /usr/bin/node --input-type=module - "$1" <<'NODE'
import {readFileSync} from 'node:fs';
const {token}=JSON.parse(readFileSync('/etc/claude-cage/browser-control.json','utf8'));
const response=await fetch('http://127.0.0.1:18500/new',{method:'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:JSON.stringify({url:process.argv[2]})});
if(!response.ok)process.exit(1);
NODE
SCRIPT
sudo chmod 0755 /usr/local/bin/cage-open
sudo ln -sf /usr/local/bin/cage-open /usr/local/bin/xdg-open
sudo tee /usr/local/bin/cage-editor-start >/dev/null <<'SCRIPT'
#!/bin/sh
set -eu
workspace=$(cat /home/cage/.config/claude-cage/editor-workspace)
exec /usr/bin/code-server --bind-addr 127.0.0.1:18080 --config /home/cage/.config/claude-cage/code-server.yaml --disable-telemetry --disable-update-check --disable-workspace-trust --disable-proxy "$workspace"
SCRIPT
sudo chmod 0755 /usr/local/bin/cage-editor-start
sudo tee /etc/systemd/system/cage-editor.service >/dev/null <<'UNIT'
[Unit]
Description=Cage file editor
[Service]
User=cage
WorkingDirectory=/home/cage
Environment=HOME=/home/cage
EnvironmentFile=/etc/claude-cage/environment
ExecStart=/usr/local/bin/cage-editor-start
Restart=on-failure
RestartSec=2
TimeoutStopSec=15
UNIT
printf 'cage-browser ALL=(root) NOPASSWD: /usr/bin/systemctl restart cage-editor.service\n' | sudo tee /etc/sudoers.d/cage-editor >/dev/null
sudo chmod 0440 /etc/sudoers.d/cage-editor
sudo tee /etc/profile.d/claude-cage.sh >/dev/null <<'SCRIPT'
if [ -f /etc/claude-cage/environment ]; then
  set -a
  . /etc/claude-cage/environment
  set +a
fi
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY FTP_PROXY NO_PROXY http_proxy https_proxy all_proxy ftp_proxy no_proxy
export BROWSER=/usr/local/bin/cage-open
SCRIPT
if ! grep -q '/etc/profile.d/claude-cage.sh' /etc/zsh/zshenv; then
  echo '. /etc/profile.d/claude-cage.sh' | sudo tee -a /etc/zsh/zshenv >/dev/null
fi
sudo tee /etc/systemd/system/cage-display.service >/dev/null <<'UNIT'
[Unit]
Description=Claude Cage virtual browser display
[Service]
User=cage-browser
ExecStartPre=/usr/bin/rm -f /home/cage-browser/.vnc/display.sock
ExecStart=/usr/bin/Xtigervnc :77 -geometry 2880x1800 -depth 24 -rfbport -1 -rfbunixpath /home/cage-browser/.vnc/display.sock -rfbunixmode 0600 -nolisten tcp -localhost -SecurityTypes VncAuth -PasswordFile /home/cage-browser/.vnc/passwd
Restart=on-failure
UNIT
sudo tee /etc/claude-cage/openbox.xml >/dev/null <<'WINDOW_CONFIG'
<?xml version="1.0" encoding="UTF-8"?>
<openbox_config xmlns="http://openbox.org/3.4/rc">
<focus><focusNew>yes</focusNew><followMouse>no</followMouse></focus>
<desktops><number>1</number></desktops>
<keyboard></keyboard><mouse></mouse>
<applications><application type="normal"><decor>no</decor><maximized>yes</maximized></application></applications>
</openbox_config>
WINDOW_CONFIG
sudo tee /etc/systemd/system/cage-window-manager.service >/dev/null <<'UNIT'
[Unit]
Description=Cage window geometry and focus (no desktop shell)
Requires=cage-display.service
After=cage-display.service
[Service]
User=cage-browser
Environment=DISPLAY=:77
ExecStart=/usr/bin/openbox --config-file /etc/claude-cage/openbox.xml
Restart=on-failure
UNIT
sudo tee /etc/systemd/system/cage-browser.service >/dev/null <<'UNIT'
[Unit]
Description=Claude Cage persistent browser
Requires=cage-display.service cage-window-manager.service
After=cage-display.service cage-window-manager.service
[Service]
User=cage-browser
Environment=DISPLAY=:77
EnvironmentFile=/etc/claude-cage/environment
ExecStart=/usr/bin/node /usr/local/lib/cage-browser-agent.mjs
# Let the controller close Chromium and flush its tabs and profile before
# systemd terminates any remaining child processes.
KillMode=mixed
Restart=on-failure
RestartSec=2
UNIT
sudo tee /etc/systemd/system/cage-vnc.service >/dev/null <<'UNIT'
[Unit]
Description=Claude Cage browser screen
Requires=cage-display.service
After=cage-display.service
[Service]
User=cage-browser
ExecStart=/usr/bin/X0tigervnc -display :77 -rfbport 15900 -localhost -SecurityTypes VncAuth -PasswordFile /home/cage-browser/.vnc/passwd -AlwaysShared
Restart=on-failure
RestartSec=2
UNIT
sudo tee /etc/systemd/system/cage-webvnc.service >/dev/null <<'UNIT'
[Unit]
Description=Claude Cage browser web access
Requires=cage-vnc.service
After=cage-vnc.service
[Service]
User=cage-browser
ExecStart=/usr/bin/websockify --web=/usr/share/novnc 127.0.0.1:16080 127.0.0.1:15900
Restart=on-failure
UNIT
sudo systemctl daemon-reload

if ! grep -q 'CAGE_WORKBENCH_PROMPT' /home/cage/.bashrc; then
  sudo tee -a /home/cage/.bashrc >/dev/null <<'PROMPT'
# CAGE_WORKBENCH_PROMPT
if [ "${CAGE_WORKBENCH:-}" = 1 ]; then
  PS1='\[\e[32m\]cage\[\e[0m\]:\[\e[34m\]\W\[\e[0m\]\$ '
fi
PROMPT
fi
