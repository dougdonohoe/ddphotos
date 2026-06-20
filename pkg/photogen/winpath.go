// SPDX-License-Identifier: AGPL-3.0-only
// https://github.com/dougdonohoe/ddphotos

package photogen

import (
	"runtime"
	"strings"
)

// winToUnixPath converts a Windows-style path (as written by the native Java wizard
// into albums.yaml) into the container path that the ddphotos mounts use. The mapping
//
//	C:\Users\name\Dropbox\Photos  ->  /mnt/c/Users/name/Dropbox/Photos
//
// (drive letter lowercased under /mnt/<drive>/, backslashes turned into forward slashes)
// must stay in sync with the bash to_container_path helper in docker/ddphotos, which
// mounts the host folder at exactly this location.
//
// It is a no-op on native Windows (runtime.GOOS == "windows"), where Windows paths are
// already valid, and for paths that are not Windows-style (e.g. an ordinary Unix path
// on macOS/Linux).
func winToUnixPath(p string) string {
	if runtime.GOOS == "windows" || p == "" {
		return p
	}
	// Drive-absolute: C:\... or C:/...  ->  /mnt/c/...
	if len(p) >= 3 && isDriveLetter(p[0]) && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		rest := strings.ReplaceAll(p[3:], "\\", "/")
		return "/mnt/" + strings.ToLower(p[:1]) + "/" + rest
	}
	// Relative Windows path (backslashes only): sub\dir -> sub/dir
	if strings.Contains(p, "\\") {
		return strings.ReplaceAll(p, "\\", "/")
	}
	return p
}

// isDriveLetter reports whether c is an ASCII letter usable as a Windows drive letter.
func isDriveLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
