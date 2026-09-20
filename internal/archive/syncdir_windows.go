//go:build windows

package archive

// Windows does not support fsync on a directory handle. The file itself is
// synced before rename; ZimaOS production builds also fsync the containing
// directory through the Unix implementation.
func syncDir(_ string) error { return nil }
