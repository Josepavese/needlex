package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const EnvHome = "NEEDLEX_HOME"

func DefaultStateRoot() string {
	return StableStateRoot()
}

func InstalledStateRoot() string {
	home := os.Getenv("HOME")
	if strings.TrimSpace(home) == "" {
		if userHome, err := os.UserHomeDir(); err == nil {
			home = userHome
		}
	}
	return installedStateRootFor(runtime.GOOS, home, os.Getenv("XDG_DATA_HOME"), os.Getenv("LOCALAPPDATA"))
}

func StableStateRoot() string {
	if root := strings.TrimSpace(os.Getenv(EnvHome)); root != "" {
		return root
	}
	root := strings.TrimSpace(InstalledStateRoot())
	if root != "" && filepath.IsAbs(root) {
		return root
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		switch runtime.GOOS {
		case "windows":
			return strings.ReplaceAll(filepath.Join(home, "AppData", "Local", "NeedleX"), "/", `\`)
		default:
			return filepath.Join(home, ".needlex")
		}
	}
	return DefaultStateRoot()
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
