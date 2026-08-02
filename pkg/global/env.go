package global

import (
	"log/slog"
	"os"
	"slices"
	"strings"
)

var okenvs = map[string]bool{
	// Core System & Shell
	"USER":  true,
	"HOME":  true,
	"PATH":  true,
	"PWD":   true,
	"SHELL": true, // Added: Bash needs to know its shell context
	"TERM":  true,
	"LANG":  true,

	// Tooling (Mise/Python/Cuda)
	"__MISE_EXE":         true,
	"REQUESTS_CA_BUNDLE": true,
	"CUDA_PATH":          true, // Added: Since you have /opt/cuda in PATH

	// Orchard Specifics
	"ORCHARD_TOOLS": true, // Added: Vital for your project structure
	"ORCHARD_TREE":  true, // Added: Vital for your project structure
	"OLLAMA_HOST":   true,
	"OLLAMA_MODEL":  true,
	"OLLAMA_THINK":  true,

	// XDG/Filesystem (Standard for modern apps)
	"XDG_CACHE_HOME":           true,
	"XDG_DATA_HOME":            true,
	"XDG_CONFIG_DIRS":          true,
	"XDG_CONFIG_HOME":          true,
	"XDG_RUNTIME_DIR":          true,
	"DBUS_SESSION_BUS_ADDRESS": true,
	"WAYLAND_DISPLAY":          true,
}

func EnvDeleteFunc(s string) bool {
	x := strings.Split(s, "=")[0]
	return !okenvs[x]
}

func NewCleanEnviron() []string {
	env := slices.DeleteFunc(os.Environ(), EnvDeleteFunc)
	if !slices.ContainsFunc(env, func(e string) bool {
		return strings.HasPrefix(e, "REQUESTS_CA_BUNDLE")
	}) {
		slog.Warn("adding REQUESTS_CA_BUNDLE env")
		env = append(env, "REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt")
	}
	//for i := range env {
	//		log.Printf("using python with env %s", env[i])
	//	}
	return env
}

func SetNewEnv(env []string) {
	os.Clearenv()
	for _, e := range env {
		// SplitN with 2 ensures we only split on the first "=",
		// which is critical if the VALUE contains an "=" (like a connection string).
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 {
			_ = os.Setenv(pair[0], pair[1])
		} else {
			_ = os.Setenv(pair[0], "")
		}
	}
}

func ClearEnv() {
	os.Clearenv()
}
