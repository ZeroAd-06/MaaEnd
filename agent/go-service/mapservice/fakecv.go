package mapservice

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"
)

// ==========================================
// 基础图像工具
// ==========================================

// EnsureRGBA 将任意图像转换为 *image.RGBA
func EnsureRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			dst.Set(x, y, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

// copySubImage 从源图像的指定矩形区域创建一个新的 RGBA 图像。
func copySubImage(src *image.RGBA, r image.Rectangle) *image.RGBA {
	w, h := r.Dx(), r.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	srcStride := src.Stride
	dstStride := dst.Stride
	srcPix := src.Pix
	dstPix := dst.Pix

	srcBase := src.PixOffset(r.Min.X, r.Min.Y)

	for y := 0; y < h; y++ {
		copy(dstPix[y*dstStride:y*dstStride+w*4], srcPix[srcBase+y*srcStride:srcBase+y*srcStride+w*4])
	}

	return dst
}

// DownscaleRGBA 分配一个新图像并使用最近邻插值进行降采样。
func DownscaleRGBA(img image.Image, scale int) *image.RGBA {
	bounds := img.Bounds()
	newW := bounds.Dx() / scale
	newH := bounds.Dy() / scale
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	DownscaleRGBAInto(img, dst, scale)
	return dst
}

// DownscaleRGBAInto 使用最近邻插值将 src 降采样到 dst。
// dst 的边界必须匹配 src 边界 / 缩放比例。
func DownscaleRGBAInto(img image.Image, dst *image.RGBA, scale int) {
	bounds := img.Bounds()
	newW := dst.Bounds().Dx()
	newH := dst.Bounds().Dy()

	// RGBA 快速路径
	if src, ok := img.(*image.RGBA); ok {
		srcStride := src.Stride
		dstStride := dst.Stride
		srcPix := src.Pix
		dstPix := dst.Pix

		for y := 0; y < newH; y++ {
			// Src Y coordinate
			srcY := bounds.Min.Y + y*scale
			srcRowY := srcY - src.Rect.Min.Y
			if srcRowY < 0 {
				continue
			}
			srcRowStart := srcRowY * srcStride
			dstRowOffset := y * dstStride

			for x := 0; x < newW; x++ {
				srcX := bounds.Min.X + x*scale
				srcColX := srcX - src.Rect.Min.X
				srcOffset := srcRowStart + srcColX*4
				dstOffset := dstRowOffset + x*4
				copy(dstPix[dstOffset:dstOffset+4], srcPix[srcOffset:srcOffset+4])
			}
		}
		return
	}

	// 其他慢速路径
	for y := 0; y < newH; y++ {
		srcY := bounds.Min.Y + y*scale
		for x := 0; x < newW; x++ {
			srcX := bounds.Min.X + x*scale
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}
}

// ==========================================
// 几何与绘图
// ==========================================

func checkBounds(x, y, w, h int) bool {
	return x >= 0 && x < w && y >= 0 && y < h
}

func drawRect(dst *image.RGBA, x1, y1, x2, y2 int, c color.Color) {
	bounds := dst.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 > w {
		x2 = w
	}
	if y2 > h {
		y2 = h
	}

	// Top & Bottom
	for x := x1; x < x2; x++ {
		if checkBounds(x, y1, w, h) {
			dst.Set(x, y1, c)
		}
		if checkBounds(x, y2-1, w, h) {
			dst.Set(x, y2-1, c)
		}
	}
	// Left & Right
	for y := y1; y < y2; y++ {
		if checkBounds(x1, y, w, h) {
			dst.Set(x1, y, c)
		}
		if checkBounds(x2-1, y, w, h) {
			dst.Set(x2-1, y, c)
		}
	}
}

// GenerateCircularMask 为圆形小地图创建 Alpha 蒙版
func GenerateCircularMask(w, h int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, w, h))

	// 外半径（保留地图区域）
	radius := float64(w) / 2
	if float64(h)/2 < radius {
		radius = float64(h) / 2
	}

	// 内半径（移除玩家箭头）
	innerRadius := 10.0

	cx, cy := float64(w)/2, float64(h)/2

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x) - cx + 0.5
			dy := float64(y) - cy + 0.5
			distSq := dx*dx + dy*dy

			// 在外半径内 且 在内半径外
			if distSq <= radius*radius && distSq > innerRadius*innerRadius {
				mask.SetAlpha(x, y, color.Alpha{255}) // 有效
			} else {
				mask.SetAlpha(x, y, color.Alpha{0}) // 忽略（中心 + 边缘）
			}
		}
	}
	return mask
}

// ApplyMaskFastInto 将 Alpha 蒙版应用到图像并绘制到 dst。
func ApplyMaskFastInto(src image.Image, dst *image.RGBA, mask *image.Alpha) {
	bounds := src.Bounds()
	// 将 src 绘制到 dst
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	// 应用 alpha 蒙版
	draw.DrawMask(dst, dst.Bounds(), src, bounds.Min, mask, image.Point{}, draw.Src)
}

// ==========================================
// 图像过滤/遮罩
// ==========================================

// ApplySpotlightEffect 通过透明化移除 Tier 地图中的暗区（空白）。
func ApplySpotlightEffect(img *image.RGBA, threshold int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	pix := img.Pix
	stride := img.Stride

	for y := 0; y < h; y++ {
		rowOffset := y * stride
		for x := 0; x < w; x++ {
			offset := rowOffset + x*4

			// 如果已经是透明的，则跳过
			if pix[offset+3] == 0 {
				continue
			}

			r := int(pix[offset+0])
			g := int(pix[offset+1])
			b := int(pix[offset+2])

			luma := (r*3 + g*6 + b) / 10

			if luma < threshold {
				// 设置 Alpha 为 0（透明）
				pix[offset+3] = 0
				// 可选：清除颜色通道以便调试/保持整洁
				pix[offset+0] = 0
				pix[offset+1] = 0
				pix[offset+2] = 0
			}
		}
	}
}

// ApplyVoidFilter 扫描图像并将暗区（低于阈值）替换为透明 Alpha (0)。
func ApplyVoidFilter(img *image.RGBA, threshold int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	pix := img.Pix
	stride := img.Stride

	for y := 0; y < h; y++ {
		rowOffset := y * stride
		for x := 0; x < w; x++ {
			offset := rowOffset + x*4

			r := int(pix[offset+0])
			g := int(pix[offset+1])
			b := int(pix[offset+2])

			luma := (r*3 + g*6 + b) / 10

			if luma < threshold {
				// 设置 Alpha 为 0（透明）
				pix[offset+3] = 0
				// 清除 RGB 通道
				pix[offset+0] = 0
				pix[offset+1] = 0
				pix[offset+2] = 0
			}
		}
	}
}

// ==========================================
// 模板匹配（核心算法）
// ==========================================

type ProbePoint struct {
	X, Y    int
	R, G, B int
}

type TemplateProbe struct {
	Points []ProbePoint
	Width  int
	Height int
}

func NewTemplateProbe() *TemplateProbe {
	return &TemplateProbe{
		Points: make([]ProbePoint, 0, 4096),
	}
}

// UpdateFromMinimap
// 1. mask
// 2. maskIcons
// 3. 存入Probe
func (tp *TemplateProbe) UpdateFromMinimap(img *image.RGBA, mask *image.Alpha) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	tp.Width = w
	tp.Height = h

	tp.Points = tp.Points[:0]

	pix := img.Pix
	stride := img.Stride

	maskPix := mask.Pix
	maskStride := mask.Stride

	const DiffThreshold = 40

	for y := 0; y < h; y++ {
		rowOffset := y * stride
		maskOffset := y * maskStride

		for x := 0; x < w; x++ {
			if maskPix[maskOffset+x] == 0 {
				continue
			}

			offset := rowOffset + x*4
			if pix[offset+3] == 0 {
				continue
			}

			r := int(pix[offset+0])
			g := int(pix[offset+1])
			b := int(pix[offset+2])

			isIcon := false

			if r > 100 && g > 100 {
				minRG := r
				if g < minRG {
					minRG = g
				}
				if (minRG - b) > DiffThreshold {
					isIcon = true
				}
			}
			if !isIcon && b > 100 {
				maxRG := r
				if g > maxRG {
					maxRG = g
				}
				if (b - maxRG) > DiffThreshold {
					isIcon = true
				}
			}

			if isIcon {
				continue
			}

			tp.Points = append(tp.Points, ProbePoint{
				X: x, Y: y,
				R: r, G: g, B: b,
			})
		}
	}
}

// MatchProbe 匹配，返回最佳匹配位置和平均差异
// step: 物理步进， probeStep: 采样步进
func MatchProbe(img *image.RGBA, probe *TemplateProbe, step int, probeStep int, useConcurrency bool) (bestX, bestY int, avgDiff float64) {
	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()

	maxX := imgW - probe.Width
	maxY := imgH - probe.Height
	if maxX <= 0 || maxY <= 0 {
		return 0, 0, 0
	}

	imgPix := img.Pix
	imgStride := img.Stride
	points := probe.Points

	validPixels := len(points) / probeStep
	if validPixels == 0 {
		return 0, 0, 0
	}

	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}

	// 色调容忍度
	const ChromaThreshold = 45

	// 惩罚权重
	const ChromaWeight = 15

	matchRect := func(startX, endX, startY, endY int) (int, int, int) {
		localMinSAD := math.MaxInt64
		localX, localY := 0, 0

		for y := startY; y < endY; y += step {
			rowBase := y * imgStride
			for x := startX; x < endX; x += step {
				baseOffset := rowBase + x*4
				currentSAD := 0
				validCount := 0

				for i := 0; i < len(points); i += probeStep {
					p := &points[i]

					off := baseOffset + (p.Y * imgStride) + (p.X * 4)

					if off < 0 || off+2 >= len(imgPix) {
						continue
					}

					validCount++

					r := int(imgPix[off])
					g := int(imgPix[off+1])
					b := int(imgPix[off+2])

					diffR := abs(r - p.R)
					diffG := abs(g - p.G)
					diffB := abs(b - p.B)
					baseDiff := diffR + diffG + diffB

					pRG := p.R - p.G
					pBG := p.B - p.G
					mRG := r - g
					mBG := b - g

					chromaDiff := abs(pRG-mRG) + abs(pBG-mBG)

					// 非线性惩罚
					penalty := 0
					if chromaDiff > ChromaThreshold {
						penalty = (chromaDiff - ChromaThreshold) * ChromaWeight
					}

					currentSAD += baseDiff + penalty

					if currentSAD > localMinSAD {
						break
					}
				}

				// 边缘检查
				minRequired := (len(points) * 85) / (probeStep * 100)
				if validCount < minRequired {
					currentSAD = math.MaxInt32
					continue
				}

				if currentSAD < localMinSAD {
					localMinSAD = currentSAD
					localX = x
					localY = y
				}
			}
		}
		return localX, localY, localMinSAD
	}

	if !useConcurrency {
		bx, by, sad := matchRect(0, maxX, 0, maxY)
		return bx, by, calcAvgDiff(sad, validPixels)
	}

	var mutex sync.Mutex
	globalMinSAD := math.MaxInt64
	globalX, globalY := 0, 0
	numWorkers := 8
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	rowsPerWorker := (maxY/step + 1 + numWorkers - 1) / numWorkers

	for i := 0; i < numWorkers; i++ {
		startIdx := i * rowsPerWorker
		endIdx := (i + 1) * rowsPerWorker
		go func(sIdx, eIdx int) {
			defer wg.Done()
			yStart := sIdx * step
			yEnd := eIdx * step
			if yEnd > maxY {
				yEnd = maxY + 1
			}
			lx, ly, lSad := matchRect(0, maxX, yStart, yEnd)
			mutex.Lock()
			if lSad < globalMinSAD {
				globalMinSAD = lSad
				globalX = lx
				globalY = ly
			}
			mutex.Unlock()
		}(startIdx, endIdx)
	}
	wg.Wait()

	return globalX, globalY, calcAvgDiff(globalMinSAD, validPixels)
}

// calcAvgDiff 计算平均差异（越小越好）
func calcAvgDiff(sad int, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(sad) / float64(count*3)
}
