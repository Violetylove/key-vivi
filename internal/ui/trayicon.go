package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"fyne.io/fyne/v2"
)

// 托盘图标在运行时绘制，二进制不携带图片资源。字形是 5x7 位图放大，
// 保证 16px 下"KV"仍可辨认。
var kvGlyphs = map[rune][7]string{
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
}

const (
	iconSize  = 32
	glyphW    = 5
	glyphH    = 7
	iconScale = 2
	iconGap   = iconScale
	iconRound = 7
)

var (
	activeIcon = color.RGBA{R: 0x25, G: 0x63, B: 0xEB, A: 0xFF}
	pausedIcon = color.RGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}
	letterInk  = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	iconOnce    sync.Once
	iconCache   [2]fyne.Resource
	iconBuildEr error
)

// insideRounded 判断 (x, y) 是否落在圆角方块内。
func insideRounded(x, y, w, h, radius int) bool {
	if x < 0 || y < 0 || x >= w || y >= h {
		return false
	}
	cx, cy := x, y
	if x < radius {
		cx = radius
	} else if x >= w-radius {
		cx = w - radius - 1
	}
	if y < radius {
		cy = radius
	} else if y >= h-radius {
		cy = h - radius - 1
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

func drawTrayIcon(background color.RGBA) *image.RGBA {
	icon := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			if insideRounded(x, y, iconSize, iconSize, iconRound) {
				icon.SetRGBA(x, y, background)
			}
		}
	}
	letters := []rune("KV")
	total := len(letters)*glyphW*iconScale + (len(letters)-1)*iconGap
	originX := (iconSize - total) / 2
	originY := (iconSize - glyphH*iconScale) / 2
	for i, letter := range letters {
		glyph := kvGlyphs[letter]
		baseX := originX + i*(glyphW*iconScale+iconGap)
		for row := 0; row < glyphH; row++ {
			for col := 0; col < glyphW; col++ {
				if glyph[row][col] != '1' {
					continue
				}
				for dy := 0; dy < iconScale; dy++ {
					for dx := 0; dx < iconScale; dx++ {
						icon.SetRGBA(baseX+col*iconScale+dx, originY+row*iconScale+dy, letterInk)
					}
				}
			}
		}
	}
	return icon
}

// trayIcon 返回缓存的"KV"图标，暂停时变灰。
func trayIcon(paused bool) fyne.Resource {
	iconOnce.Do(func() {
		for i, background := range []color.RGBA{activeIcon, pausedIcon} {
			var buffer bytes.Buffer
			if err := png.Encode(&buffer, drawTrayIcon(background)); err != nil {
				iconBuildEr = err
				return
			}
			name := "kv-active.png"
			if i == 1 {
				name = "kv-paused.png"
			}
			iconCache[i] = fyne.NewStaticResource(name, buffer.Bytes())
		}
	})
	if paused {
		return iconCache[1]
	}
	return iconCache[0]
}
