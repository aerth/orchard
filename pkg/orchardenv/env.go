// orchardenv cleans up the environment
package orchardenv

import (
	"os"
	"slices"
)

var okkeys = []string{
	"XDG_CONFIG_DIRS",
	"XDG_SESSION_PATH",
	"XDG_MENU_PREFIX",
	"XDG_BACKEND",
	"XDG_DATA_HOME",
	"XDG_CONFIG_HOME",
	"XDG_SEAT",
	"XDG_SESSION_DESKTOP",
	"XDG_SESSION_TYPE",
	"XDG_CURRENT_DESKTOP",
	"XDG_SEAT_PATH",
	"XDG_CACHE_HOME",
	"XDG_SESSION_CLASS",
	"XDG_VTNR",
	"XDG_SESSION_ID",
	"XDG_STATE_HOME",
	"XDG_RUNTIME_DIR",
	"XDG_DATA_DIRS",
	"GPG_TTY",
	"WINDOWID",
	"PATH",
	"HOME",
	"USER",
	"PWD",
	"SHELL",
	"EDITOR",
	"VISUAL",
	"LANG",
	"TERM",
	"WAYLAND_DISPLAY",
	"DISPLAY",
	"DBUS_SESSION_BUS_ADDRESS",
	"CUDA_PATH",
	"NVCC_CCBIN",
	"PYTHONPATH",
	"NODE_ENV",
}

// OKENVNAMES is list of env var names (K)
func OKENVNAMES() []string {
	return slices.Clone(okkeys)
}

// OKENVA is environ
func OKENVA() []string {
	envmap := OKENVMAP()
	eenv := make([]string, len(envmap))
	i := 0
	for k, v := range envmap {
		eenv[i] = k + "=" + v
		i++
	}
	return eenv
}

// OKENVMAP is environ as map
func OKENVMAP() map[string]string {
	envmap := map[string]string{}
	for _, v := range OKENVNAMES() {
		if _, found := envmap[v]; !found { // allow dot env to override per-tool
			e := os.Getenv(v)
			if e != "" {
				envmap[v] = e
			}

		}
	}
	return envmap
}
