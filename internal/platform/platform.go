package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const EnvHome = "NEEDLEX_HOME"

func DefaultStateRoot() string {
	if root := strings.TrimSpace(os.Getenv(EnvHome)); root != "" {
		return root
	}
	return ".needlex"
}

func InstalledStateRoot() string {
	return installedStateRootFor(runtime.GOOS, os.Getenv("HOME"), os.Getenv("XDG_DATA_HOME"), os.Getenv("LOCALAPPDATA"))
}

func installedStateRootFor(goos, home, xdgDataHome, localAppData string) string {
	switch strings.TrimSpace(goos) {
	case "windows":
		base := strings.TrimSpace(localAppData)
		if base == "" {
			base = filepath.Join(strings.TrimSpace(home), "AppData", "Local")
		}
		return strings.ReplaceAll(filepath.Join(base, "NeedleX"), "/", `\`)
	case "darwin":
		return filepath.Join(strings.TrimSpace(home), "Library", "Application Support", "NeedleX")
	default:
		base := strings.TrimSpace(xdgDataHome)
		if base == "" {
			base = filepath.Join(strings.TrimSpace(home), ".local", "share")
		}
		return filepath.Join(base, "needlex")
	}
}
