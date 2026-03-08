package imageproc

import (
	"fmt"
	"image"
	"os"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// DecodedImage contains decoded image pixels and basic metadata.
type DecodedImage struct {
	Image  image.Image
	Format string
	Width  int
	Height int
}

// DecodeFile decodes an image file using Go's registered image decoders.
func DecodeFile(filePath string) (DecodedImage, error) {
	if filePath == "" {
		return DecodedImage{}, fmt.Errorf("file path is required")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return DecodedImage{}, fmt.Errorf("open image file: %w", err)
	}
	defer file.Close()

	decoded, format, err := image.Decode(file)
	if err != nil {
		return DecodedImage{}, fmt.Errorf("decode image file: %w", err)
	}

	bounds := decoded.Bounds()
	return DecodedImage{
		Image:  decoded,
		Format: format,
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
	}, nil
}
