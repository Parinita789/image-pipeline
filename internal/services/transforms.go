package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"

	"image-pipeline/internal/models"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	xdraw "golang.org/x/image/draw"
)

var allowedTransforms = map[string]bool{
	"grayscale": true,
	"sepia":     true,
	"blur":      true,
	"sharpen":   true,
	"invert":    true,
	"remove-bg": true,
	"resize":    true,
	"crop":      true,
	"watermark": true,
	"format":    true,
}

func IsValidTransform(t models.TransformConfig) error {
	if !allowedTransforms[t.Type] {
		return fmt.Errorf("unknown transform: %s", t.Type)
	}
	switch t.Type {
	case "resize":
		if t.Width <= 0 && t.Height <= 0 {
			return fmt.Errorf("resize requires width or height > 0")
		}
	case "crop":
		if t.Width <= 0 || t.Height <= 0 {
			return fmt.Errorf("crop requires width and height > 0")
		}
	case "watermark":
		if t.Text == "" && t.LogoURL == "" {
			return fmt.Errorf("watermark requires text or logoUrl")
		}
		switch t.Position {
		case "", "bottom-right", "bottom-left", "top-right", "top-left":
		default:
			return fmt.Errorf("invalid position: %s", t.Position)
		}
	case "format":
		switch t.Format {
		case "jpeg", "png":
		default:
			return fmt.Errorf("unsupported format: %s (use jpeg or png)", t.Format)
		}
	}
	return nil
}

// OutputFormat returns the target format if a format transform is present, empty string otherwise.
func OutputFormat(transforms []models.TransformConfig) string {
	for _, t := range transforms {
		if t.Type == "format" {
			return t.Format
		}
	}
	return ""
}

func Grayscale(img image.Image) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	sp, dp := src.Pix, out.Pix

	for y := 0; y < h; y++ {
		si := y * src.Stride
		di := y * out.Stride
		for x := 0; x < w; x++ {
			off := x * 4
			r, g, bl := sp[si+off], sp[si+off+1], sp[si+off+2]
			gray := uint8((19595*uint32(r) + 38470*uint32(g) + 7471*uint32(bl) + 1<<15) >> 16)
			dp[di+off] = gray
			dp[di+off+1] = gray
			dp[di+off+2] = gray
			dp[di+off+3] = sp[si+off+3]
		}
	}
	return out
}

func Sepia(img image.Image) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	sp, dp := src.Pix, out.Pix

	for y := 0; y < h; y++ {
		si := y * src.Stride
		di := y * out.Stride
		for x := 0; x < w; x++ {
			off := x * 4
			r, g, bl := uint32(sp[si+off]), uint32(sp[si+off+1]), uint32(sp[si+off+2])
			sr := (r*393 + g*769 + bl*189) >> 10
			sg := (r*349 + g*686 + bl*168) >> 10
			sb := (r*272 + g*534 + bl*131) >> 10
			if sr > 255 {
				sr = 255
			}
			if sg > 255 {
				sg = 255
			}
			if sb > 255 {
				sb = 255
			}
			dp[di+off] = uint8(sr)
			dp[di+off+1] = uint8(sg)
			dp[di+off+2] = uint8(sb)
			dp[di+off+3] = sp[si+off+3]
		}
	}
	return out
}

// Blur applies a separable box blur with radius 8 (17×17 effective kernel).
func Blur(img image.Image) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	radius := 8

	tmp := image.NewRGBA(b)
	out := image.NewRGBA(b)

	// Horizontal pass
	for y := 0; y < h; y++ {
		si := y * src.Stride
		di := y * tmp.Stride
		var rSum, gSum, bSum, aSum uint32
		count := uint32(0)

		for kx := 0; kx <= radius && kx < w; kx++ {
			off := kx * 4
			rSum += uint32(src.Pix[si+off])
			gSum += uint32(src.Pix[si+off+1])
			bSum += uint32(src.Pix[si+off+2])
			aSum += uint32(src.Pix[si+off+3])
			count++
		}

		for x := 0; x < w; x++ {
			off := x * 4
			tmp.Pix[di+off] = uint8(rSum / count)
			tmp.Pix[di+off+1] = uint8(gSum / count)
			tmp.Pix[di+off+2] = uint8(bSum / count)
			tmp.Pix[di+off+3] = uint8(aSum / count)

			nx := x + radius + 1
			if nx < w {
				noff := nx * 4
				rSum += uint32(src.Pix[si+noff])
				gSum += uint32(src.Pix[si+noff+1])
				bSum += uint32(src.Pix[si+noff+2])
				aSum += uint32(src.Pix[si+noff+3])
				count++
			}
			ox := x - radius
			if ox >= 0 {
				ooff := ox * 4
				rSum -= uint32(src.Pix[si+ooff])
				gSum -= uint32(src.Pix[si+ooff+1])
				bSum -= uint32(src.Pix[si+ooff+2])
				aSum -= uint32(src.Pix[si+ooff+3])
				count--
			}
		}
	}

	// Vertical pass
	for x := 0; x < w; x++ {
		var rSum, gSum, bSum, aSum uint32
		count := uint32(0)

		xoff := x * 4
		for ky := 0; ky <= radius && ky < h; ky++ {
			si := ky*tmp.Stride + xoff
			rSum += uint32(tmp.Pix[si])
			gSum += uint32(tmp.Pix[si+1])
			bSum += uint32(tmp.Pix[si+2])
			aSum += uint32(tmp.Pix[si+3])
			count++
		}

		for y := 0; y < h; y++ {
			di := y*out.Stride + xoff
			out.Pix[di] = uint8(rSum / count)
			out.Pix[di+1] = uint8(gSum / count)
			out.Pix[di+2] = uint8(bSum / count)
			out.Pix[di+3] = uint8(aSum / count)

			ny := y + radius + 1
			if ny < h {
				si := ny*tmp.Stride + xoff
				rSum += uint32(tmp.Pix[si])
				gSum += uint32(tmp.Pix[si+1])
				bSum += uint32(tmp.Pix[si+2])
				aSum += uint32(tmp.Pix[si+3])
				count++
			}
			oy := y - radius
			if oy >= 0 {
				si := oy*tmp.Stride + xoff
				rSum -= uint32(tmp.Pix[si])
				gSum -= uint32(tmp.Pix[si+1])
				bSum -= uint32(tmp.Pix[si+2])
				aSum -= uint32(tmp.Pix[si+3])
				count--
			}
		}
	}
	return out
}

func Sharpen(img image.Image) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	sp, dp := src.Pix, out.Pix
	stride := src.Stride

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			di := y*stride + x*4
			if y == 0 || y == h-1 || x == 0 || x == w-1 {
				copy(dp[di:di+4], sp[di:di+4])
				continue
			}
			ci := di
			ti := (y-1)*stride + x*4
			bi := (y+1)*stride + x*4
			li := y*stride + (x-1)*4
			ri := y*stride + (x+1)*4

			for c := 0; c < 3; c++ {
				v := 5*int(sp[ci+c]) - int(sp[ti+c]) - int(sp[bi+c]) - int(sp[li+c]) - int(sp[ri+c])
				if v < 0 {
					v = 0
				} else if v > 255 {
					v = 255
				}
				dp[di+c] = uint8(v)
			}
			dp[di+3] = sp[di+3]
		}
	}
	return out
}

func Invert(img image.Image) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	sp, dp := src.Pix, out.Pix

	for y := 0; y < h; y++ {
		si := y * src.Stride
		di := y * out.Stride
		for x := 0; x < w; x++ {
			off := x * 4
			dp[di+off] = 255 - sp[si+off]
			dp[di+off+1] = 255 - sp[si+off+1]
			dp[di+off+2] = 255 - sp[si+off+2]
			dp[di+off+3] = sp[si+off+3]
		}
	}
	return out
}

// scriptDir returns the path to the scripts directory relative to this source file.
func scriptDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "scripts")
}

func RemoveBackground(img image.Image) *image.RGBA {
	// Encode image as PNG to send to rembg via stdin
	var inBuf bytes.Buffer
	if err := png.Encode(&inBuf, img); err != nil {
		return toRGBA(img)
	}

	script := filepath.Join(scriptDir(), "rembg_worker.py")
	cmd := exec.Command("python3", script)
	cmd.Stdin = &inBuf

	outData, err := cmd.Output()
	if err != nil {
		// Fallback: return original image unchanged
		return toRGBA(img)
	}

	result, _, err := image.Decode(bytes.NewReader(outData))
	if err != nil {
		return toRGBA(img)
	}

	return toRGBA(result)
}

func Resize(img image.Image, width, height int) *image.RGBA {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	if width <= 0 && height <= 0 {
		return toRGBA(img)
	}
	if width <= 0 {
		width = srcW * height / srcH
	}
	if height <= 0 {
		height = srcH * width / srcW
	}

	out := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, b, xdraw.Over, nil)
	return out
}

// Crop extracts a rectangular region from the image.
func Crop(img image.Image, x, y, width, height int) *image.RGBA {
	b := img.Bounds()
	// Clamp to image bounds
	if x < b.Min.X {
		x = b.Min.X
	}
	if y < b.Min.Y {
		y = b.Min.Y
	}
	if x+width > b.Max.X {
		width = b.Max.X - x
	}
	if y+height > b.Max.Y {
		height = b.Max.Y - y
	}
	if width <= 0 || height <= 0 {
		return toRGBA(img)
	}

	cropRect := image.Rect(x, y, x+width, y+height)
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(out, out.Bounds(), img, cropRect.Min, draw.Src)
	return out
}

func Watermark(img image.Image, text, position, logoURL string) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)

	if position == "" {
		position = "bottom-right"
	}

	// If logo URL is provided, use logo watermark
	if logoURL != "" {
		watermarkWithLogo(out, logoURL, position)
		return out
	}

	watermarkWithText(out, text, position)
	return out
}

func watermarkWithText(out *image.RGBA, text, position string) {
	b := out.Bounds()
	imgW, imgH := b.Dx(), b.Dy()

	const glyphW, glyphH = 7, 13
	textPxW := len(text) * glyphW
	if textPxW == 0 {
		return
	}

	// Scale so text is roughly 1/4 of image width
	scale := imgW / (textPxW * 4)
	if scale < 2 {
		scale = 2
	}

	// Render text at basicfont size onto a small canvas
	small := image.NewRGBA(image.Rect(0, 0, textPxW, glyphH))
	face := basicfont.Face7x13
	wmCol := color.NRGBA{R: 255, G: 255, B: 255, A: 180}

	d := &font.Drawer{
		Dst:  small,
		Src:  image.NewUniform(wmCol),
		Face: face,
		Dot:  fixed.P(0, glyphH-1),
	}
	d.DrawString(text)

	// Calculate placement with padding
	scaledW := textPxW * scale
	scaledH := glyphH * scale
	padding := imgW / 30
	if padding < 10 {
		padding = 10
	}

	var ox, oy int
	switch position {
	case "top-left":
		ox, oy = padding, padding
	case "top-right":
		ox, oy = imgW-scaledW-padding, padding
	case "bottom-left":
		ox, oy = padding, imgH-scaledH-padding
	default: // bottom-right
		ox, oy = imgW-scaledW-padding, imgH-scaledH-padding
	}

	// Scale up and alpha-blend each text pixel as a block
	blendBlock(out, small, ox, oy, scale)
}

func watermarkWithLogo(out *image.RGBA, logoURL, position string) {
	resp, err := http.Get(logoURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	logoImg, _, err := image.Decode(resp.Body)
	if err != nil {
		return
	}

	logo := toRGBA(logoImg)
	b := out.Bounds()
	imgW, imgH := b.Dx(), b.Dy()
	logoW, logoH := logo.Bounds().Dx(), logo.Bounds().Dy()

	targetW := imgW * 15 / 100
	if targetW < 30 {
		targetW = 30
	}
	targetH := logoH * targetW / logoW
	if targetH > imgH/4 {
		targetH = imgH / 4
		targetW = logoW * targetH / logoH
	}

	scaled := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), logo, logo.Bounds(), xdraw.Over, nil)

	padding := imgW / 30
	if padding < 10 {
		padding = 10
	}

	var ox, oy int
	switch position {
	case "top-left":
		ox, oy = padding, padding
	case "top-right":
		ox, oy = imgW-targetW-padding, padding
	case "bottom-left":
		ox, oy = padding, imgH-targetH-padding
	default:
		ox, oy = imgW-targetW-padding, imgH-targetH-padding
	}

	// Alpha-blend the scaled logo onto the output
	draw.Draw(out, image.Rect(ox, oy, ox+targetW, oy+targetH), scaled, image.Point{}, draw.Over)
}

// blendBlock scales up a small RGBA image by the given factor and alpha-blends
// each pixel as a block onto dst at offset (ox, oy).
func blendBlock(dst, src *image.RGBA, ox, oy, scale int) {
	db := dst.Bounds()
	dstW, dstH := db.Dx(), db.Dy()
	sp := src.Pix
	dp := dst.Pix
	outStride := dst.Stride
	srcH := src.Bounds().Dy()
	srcW := src.Bounds().Dx()

	for sy := 0; sy < srcH; sy++ {
		for sx := 0; sx < srcW; sx++ {
			si := sy*src.Stride + sx*4
			sa := uint32(sp[si+3])
			if sa == 0 {
				continue
			}
			sr, sg, sb := uint32(sp[si]), uint32(sp[si+1]), uint32(sp[si+2])
			invA := 255 - sa

			dyStart := oy + sy*scale
			dyEnd := dyStart + scale
			if dyEnd > dstH {
				dyEnd = dstH
			}
			dxStart := ox + sx*scale
			dxEnd := dxStart + scale
			if dxEnd > dstW {
				dxEnd = dstW
			}

			for dy := dyStart; dy < dyEnd; dy++ {
				if dy < 0 {
					continue
				}
				for dx := dxStart; dx < dxEnd; dx++ {
					if dx < 0 {
						continue
					}
					di := dy*outStride + dx*4
					da := uint32(dp[di+3])
					dr, dg, dbl := uint32(dp[di]), uint32(dp[di+1]), uint32(dp[di+2])
					dp[di] = uint8((sr*sa + dr*invA) / 255)
					dp[di+1] = uint8((sg*sa + dg*invA) / 255)
					dp[di+2] = uint8((sb*sa + dbl*invA) / 255)
					dp[di+3] = uint8(sa + (da*invA)/255)
				}
			}
		}
	}
}

func ApplyTransforms(img image.Image, transforms []models.TransformConfig) (image.Image, error) {
	current := img
	for _, t := range transforms {
		switch t.Type {
		case "grayscale":
			current = Grayscale(current)
		case "sepia":
			current = Sepia(current)
		case "blur":
			current = Blur(current)
		case "sharpen":
			current = Sharpen(current)
		case "invert":
			current = Invert(current)
		case "remove-bg":
			current = RemoveBackground(current)
		case "resize":
			current = Resize(current, t.Width, t.Height)
		case "crop":
			current = Crop(current, t.X, t.Y, t.Width, t.Height)
		case "watermark":
			current = Watermark(current, t.Text, t.Position, t.LogoURL)
		case "format":
			// Handled at encoding stage, not pixel manipulation
			continue
		default:
			return nil, fmt.Errorf("unknown transform: %s", t.Type)
		}
	}
	return current, nil
}

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	return rgba
}
