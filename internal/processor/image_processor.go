package processor

import (
	"bytes"
	"io"

	"github.com/disintegration/imaging"
)

func ResizeImage(input io.Reader) (*bytes.Buffer, error) {
	img, err := imaging.Decode(input)
	if err != nil {
		return nil, err
	}

	thumd := imaging.Resize(img, 800, 0, imaging.Lanczos)

	buf := new(bytes.Buffer)
	err = imaging.Encode(buf, thumd, imaging.JPEG)
	if err != nil {
		return nil, err
	}

	return buf, nil
}
