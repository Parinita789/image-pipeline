package services

import (
	"fmt"
	"image"
	"image/draw"
)

var allowedTransforms = map[string]bool{
	"grayscale": true,
	"sepia":     true,
	"blur":      true,
	"sharpen":   true,
	"invert":    true,
}

func IsValidTransform(name string) bool {
	return allowedTransforms[name]
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
			// Standard sepia matrix
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
// Two 1D passes (horizontal then vertical) = O(n) regardless of radius.
func Blur(img image.Image) *image.RGBA {
	src := toRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	radius := 8

	// Intermediate buffer after horizontal pass
	tmp := image.NewRGBA(b)
	out := image.NewRGBA(b)

	// Horizontal pass
	for y := 0; y < h; y++ {
		si := y * src.Stride
		di := y * tmp.Stride
		var rSum, gSum, bSum, aSum uint32
		count := uint32(0)

		// Seed the window with [0, radius]
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

			// Add right edge
			nx := x + radius + 1
			if nx < w {
				noff := nx * 4
				rSum += uint32(src.Pix[si+noff])
				gSum += uint32(src.Pix[si+noff+1])
				bSum += uint32(src.Pix[si+noff+2])
				aSum += uint32(src.Pix[si+noff+3])
				count++
			}
			// Remove left edge
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
				// Border pixels: copy as-is
				copy(dp[di:di+4], sp[di:di+4])
				continue
			}
			// Center pixel × 5, minus 4 neighbors
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

func ApplyTransforms(img image.Image, names []string) (image.Image, error) {
	current := img
	for _, name := range names {
		switch name {
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
		default:
			return nil, fmt.Errorf("unknown transform: %s", name)
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
