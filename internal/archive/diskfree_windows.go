//go:build windows

package archive

func diskFree(_ string) (uint64, error) {
	// Production runs on Linux/ZimaOS. Windows development still enforces the
	// configured soft quota while reporting effectively unlimited free space.
	return ^uint64(0), nil
}
