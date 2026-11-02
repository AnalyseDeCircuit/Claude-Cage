# Functions are shared by first installation and an explicit in-place upgrade.
# Each invocation owns its staging files and leaves caller traps unchanged.
cage_install_tool() (
  set -euo pipefail
  local tool="$1" arch current='' wanted url checksum artifact
  arch="$(dpkg --print-architecture)"
  case "$arch" in arm64|amd64) ;; *) echo 'Cage tool packages support arm64 and amd64.' >&2; exit 1 ;; esac
  case "$tool" in
    code-server)
      wanted="$CAGE_CODE_SERVER_VERSION"
      if command -v code-server >/dev/null; then current="$(code-server --version | head -n1 | cut -d' ' -f1)"; fi
      url="https://github.com/coder/code-server/releases/download/v$wanted/code-server_${wanted}_${arch}.deb"
      if [ "$arch" = arm64 ]; then checksum="$CAGE_CODE_SERVER_ARM64_SHA256"; else checksum="$CAGE_CODE_SERVER_AMD64_SHA256"; fi
      artifact=code-server.deb
      ;;
    mise)
      wanted="$CAGE_MISE_VERSION"
      if [ -x "$HOME/.local/bin/mise" ]; then current="$("$HOME/.local/bin/mise" --version | head -n1 | cut -d' ' -f1)"; fi
      if [ "$arch" = arm64 ]; then checksum="$CAGE_MISE_ARM64_SHA256"; else checksum="$CAGE_MISE_AMD64_SHA256"; fi
      url="https://github.com/jdx/mise/releases/download/v$wanted/mise-v$wanted-linux-$(if [ "$arch" = arm64 ]; then echo arm64; else echo x64; fi).tar.gz"
      artifact=mise.tar.gz
      ;;
    claude)
      wanted="$CAGE_CLAUDE_VERSION"
      if command -v claude >/dev/null; then current="$(DISABLE_AUTOUPDATER=1 claude --version | head -n1 | cut -d' ' -f1)"; fi
      ;;
    *) echo 'Unknown Cage tool.' >&2; exit 2 ;;
  esac
  if [ -n "$current" ] && dpkg --compare-versions "${current#v}" ge "$wanted"; then
    printf '%s already meets required version %s.\n' "$tool" "$wanted"
    exit 0
  fi
  if [ "$tool" = claude ]; then
    npm install --prefix "$HOME/.local" --global "@anthropic-ai/claude-code@$wanted" </dev/null
    exit 0
  fi
  stage="$(mktemp -d)"
  trap 'rm -rf "$stage"' EXIT
  curl --proto '=https' --tlsv1.2 --fail --location --retry 3 --connect-timeout 15 --max-time 600 "$url" -o "$stage/$artifact"
  printf '%s  %s\n' "$checksum" "$stage/$artifact" | sha256sum --check --status || { echo 'Tool checksum mismatch; installation refused.' >&2; exit 1; }
  if [ "$tool" = code-server ]; then
    sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$stage/$artifact" </dev/null
  else
    tar -xzf "$stage/$artifact" -C "$stage"
    mkdir -p "$HOME/.local/bin"
    install -m 0755 "$stage/mise/bin/mise" "$HOME/.local/bin/.mise-new"
    mv -f "$HOME/.local/bin/.mise-new" "$HOME/.local/bin/mise"
  fi
)
