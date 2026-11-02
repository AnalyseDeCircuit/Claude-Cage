package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAppBuildStopsWhenLicenseCheckFails(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS application build")
	}
	root := t.TempDir()
	tools := filepath.Join(root, "scripts", "tools")
	bin := filepath.Join(root, "bin")
	for _, directory := range []string{tools, bin} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile("../scripts/tools/build-app.sh")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		filepath.Join(tools, "build-app.sh"):        script,
		filepath.Join(root, "VERSION"):              []byte("1.0.0\n"),
		filepath.Join(bin, "make"):                  []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> calls\nexit 1\n"),
		filepath.Join(tools, "fetch-proxy-core.sh"): []byte("#!/bin/sh\nprintf 'proxy download\\n' >> calls\nexit 1\n"),
	} {
		if err := os.WriteFile(name, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", filepath.Join(tools, "build-app.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CAGE_BUILD_OUTPUT="+filepath.Join(root, "output", "Cage.app"), "CAGE_BUILD_NUMBER=1")
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("build continued after the license check failed: %s", output)
	}
	calls, err := os.ReadFile(filepath.Join(root, "calls"))
	if err != nil || string(calls) != "check-licenses\n" {
		t.Fatalf("license check did not stop the build before downloads: %q, %v", calls, err)
	}
}

// tests/setup-script.sh: the setup script's blocks, run against stubs.
func TestSetupScript(t *testing.T) {
	runScript(t, "setup-script.sh")
}

// tests/installer.sh: the curl installer and agent-vm.sh, against stub
// releases and a stub clone.
func TestInstaller(t *testing.T) {
	runScript(t, "installer.sh")
}

// tests/release.sh: scripts/tools/upstream/release.sh, against a throwaway repository.
func TestRelease(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	runScript(t, "release.sh")
}

func runScript(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("runs in a Linux guest, or on macOS and Linux")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command(bash, script).CombinedOutput()
	if err != nil {
		t.Errorf("%v\n%s", err, out)
	}
}
