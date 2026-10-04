package platform

import "math"

// RegionPosition 按九宫格定位固定区域，再把实际位图贴到区域底部。
// anchor 按从左上到右下的行优先顺序编号；偏移为物理像素。
func RegionPosition(left, top, workWidth, workHeight, imageWidth, imageHeight, regionHeight, anchor, offsetX, offsetY int) (int, int) {
	width := min(max(1, imageWidth), max(1, workWidth))
	height := min(max(1, regionHeight), max(1, workHeight))
	anchor = min(8, max(0, anchor))
	x := left + int(math.Round(float64(workWidth-width)*float64(anchor%3)/2)) + offsetX
	y := top + int(math.Round(float64(workHeight-height)*float64(anchor/3)/2)) + offsetY
	x = max(left, min(x, left+workWidth-width))
	y = max(top, min(y, top+workHeight-height))
	return x, y + height - min(imageHeight, workHeight)
}
