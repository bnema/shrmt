package gtk

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const layerShellLibrarySubstring = "libgtk4-layer-shell"

var layerShellLibraryCandidates = []string{
	"/usr/lib/libgtk4-layer-shell.so.0",
	"/usr/lib64/libgtk4-layer-shell.so.0",
	"/usr/lib/aarch64-linux-gnu/libgtk4-layer-shell.so.0",
	"/usr/lib/x86_64-linux-gnu/libgtk4-layer-shell.so.0",
	"/lib/x86_64-linux-gnu/libgtk4-layer-shell.so.0",
}

func prepareRuntime() {
	maybeReexecWithLayerShell()
	runtime.LockOSThread()
}

func maybeReexecWithLayerShell() {
	env := currentEnvMap()
	if env["WAYLAND_DISPLAY"] == "" {
		return
	}
	if strings.Contains(env["LD_PRELOAD"], layerShellLibrarySubstring) {
		return
	}
	libraryPath := findLayerShellLibrary(env)
	if libraryPath == "" {
		return
	}
	env = preloadEnv(env, libraryPath)
	envSlice := make([]string, 0, len(env))
	for key, value := range env {
		envSlice = append(envSlice, key+"="+value)
	}
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "shrmt: resolve executable for layer-shell preload: %v\n", err)
		return
	}
	if err := syscall.Exec(execPath, os.Args, envSlice); err != nil {
		fmt.Fprintf(os.Stderr, "shrmt: re-exec with layer-shell preload failed: %v\n", err)
	}
}

func currentEnvMap() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func findLayerShellLibrary(env map[string]string) string {
	seen := make(map[string]struct{}, len(layerShellLibraryCandidates))
	for _, candidate := range layerShellLibraryCandidates {
		seen[candidate] = struct{}{}
		if fileExists(candidate) {
			return candidate
		}
	}
	for _, dir := range strings.FieldsFunc(env["LD_LIBRARY_PATH"], func(r rune) bool { return r == ':' }) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "libgtk4-layer-shell.so.0")
		if _, ok := seen[candidate]; ok {
			continue
		}
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func preloadEnv(env map[string]string, libraryPath string) map[string]string {
	cloned := make(map[string]string, len(env)+1)
	for key, value := range env {
		cloned[key] = value
	}
	if current := cloned["LD_PRELOAD"]; current != "" {
		cloned["LD_PRELOAD"] = libraryPath + " " + current
	} else {
		cloned["LD_PRELOAD"] = libraryPath
	}
	return cloned
}

func fileExists(path string) bool {
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}
