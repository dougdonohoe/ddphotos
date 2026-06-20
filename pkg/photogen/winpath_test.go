// SPDX-License-Identifier: AGPL-3.0-only
// https://github.com/dougdonohoe/ddphotos

package photogen

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWinToUnixPath(t *testing.T) {
	t.Parallel()

	// These cases assume a non-Windows host (the container photogen runs in). On native
	// Windows winToUnixPath is a no-op, so skip — the GOOS=="windows" short-circuit can't
	// be exercised off-Windows.
	if runtime.GOOS == "windows" {
		t.Skip("winToUnixPath is a no-op on native Windows")
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"drive backslash", `C:\Users\xboxl\Dropbox\Photos`, "/mnt/c/Users/xboxl/Dropbox/Photos"},
		{"drive forward slash", "D:/data/pics", "/mnt/d/data/pics"},
		{"drive lowercased", `E:\Stuff`, "/mnt/e/Stuff"},
		{"relative windows", `sub\dir\file.jpg`, "sub/dir/file.jpg"},
		{"unix absolute unchanged", "/Users/example/Photos", "/Users/example/Photos"},
		{"relative unix unchanged", "2025-10 Girona", "2025-10 Girona"},
		{"empty unchanged", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, winToUnixPath(tc.in))
		})
	}
}
