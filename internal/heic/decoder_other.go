//go:build !darwin

package heic

func convertPlatform(heicPath, jpegPath string, _ bool) error {
	return convertFFmpeg(heicPath, jpegPath)
}
