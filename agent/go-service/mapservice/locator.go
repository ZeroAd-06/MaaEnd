package mapservice

import (
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

const (
	MaxLostTrackingCount = 3
	MinMatchScore        = 0.7
	MobileSearchRadius   = 50.0
)

type MapLocator struct {
	basePath     string
	lastKnownPos *MapPosition
	lostTracking int

	// 多区域支持
	zones         map[string]*image.RGBA
	currentZoneID string

	velocityX float64 // px/s
	velocityY float64 // px/s
	lastTime  time.Time

	maskAlpha *image.Alpha

	// Buffers
	workBuffer   *image.RGBA // 用于遮罩处理的小地图
	searchBuffer *image.RGBA // 用于搜索操作的复用缓冲区

	// sharedProbe 复用Probe对象
	sharedProbe *TemplateProbe
}

func NewMapLocator(zoneConfigs map[string]string) (*MapLocator, error) {
	loc := &MapLocator{
		zones:        make(map[string]*image.RGBA),
		lostTracking: MaxLostTrackingCount + 1, // 初始判定为丢失
		sharedProbe:  NewTemplateProbe(),       // 初始化一次
	}

	for id, path := range zoneConfigs {
		log.Info().Str("zone", id).Str("path", path).Msg("Loading map zone...")
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open map zone %s: %w", id, err)
		}

		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode map zone %s: %w", id, err)
		}

		// Ensure RGBA
		fullRGBA := EnsureRGBA(img)

		// Tier 地图遮罩
		isTierMap := !strings.HasSuffix(id, "_Base")
		if isTierMap {
			log.Info().Str("zone", id).Msg("Applying spotlight mask to Tier map...")
		} else {
			// 透明化
			log.Info().Str("zone", id).Msg("Applying Void Filter (Transparent to Base map...")
			ApplyVoidFilter(fullRGBA, 30) // Base地图过滤暗部阈值
		}

		loc.zones[id] = fullRGBA
		log.Info().Str("zone", id).Int("w", fullRGBA.Bounds().Dx()).Int("h", fullRGBA.Bounds().Dy()).Msg("Zone loaded")

		// 参考路径
		if loc.basePath == "" {
			loc.basePath = filepath.Dir(path)
		}
	}

	if len(loc.zones) == 0 {
		return nil, fmt.Errorf("no map zones loaded")
	}

	// 预计算圆形遮罩
	loc.maskAlpha = GenerateCircularMask(MinimapROIWidth, MinimapROIHeight)

	return loc, nil
}

func (m *MapLocator) Close() {
	m.zones = nil
}

func (m *MapLocator) Locate(ctx *maa.Context, minimap image.Image) (*MapPosition, error) {
	// 确保小地图为 RGBA 并应用遮罩
	bounds := minimap.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// 检查遮罩尺寸
	if m.maskAlpha == nil || m.maskAlpha.Bounds().Dx() != w || m.maskAlpha.Bounds().Dy() != h {
		m.maskAlpha = GenerateCircularMask(w, h)
	}

	// 缓冲区复用：workBuffer
	if m.workBuffer == nil || m.workBuffer.Bounds().Dx() != w || m.workBuffer.Bounds().Dy() != h {
		m.workBuffer = image.NewRGBA(image.Rect(0, 0, w, h))
	}

	// 复制 minimap 到 workBuffer
	draw.Draw(m.workBuffer, m.workBuffer.Bounds(), minimap, bounds.Min, draw.Src)

	// 单次遍历生成 Probe (遮罩+过滤)
	m.sharedProbe.UpdateFromMinimap(m.workBuffer, m.maskAlpha)

	now := time.Now()

	// ---------------------------------------------------------
	// 追踪模式 (Tracking Mode)
	// ---------------------------------------------------------
	if m.currentZoneID != "" && m.lastKnownPos != nil && m.lostTracking <= MaxLostTrackingCount {
		zoneImg, ok := m.zones[m.currentZoneID]
		if ok {
			// 运动预测
			dt := now.Sub(m.lastTime).Seconds()
			if dt > 0.5 {
				dt = 0
				m.velocityX = 0
				m.velocityY = 0
			}
			predX := m.lastKnownPos.X + m.velocityX*dt
			predY := m.lastKnownPos.Y + m.velocityY*dt

			// 定义搜索矩形，稀疏搜索
			cx, cy := int(predX), int(predY)
			r := int(MobileSearchRadius)
			pad := r + (w+h)/2
			fullBounds := zoneImg.Bounds()
			searchRect := image.Rect(cx-pad, cy-pad, cx+pad, cy+pad).Intersect(fullBounds)

			if !searchRect.Empty() {
				// Copy ROI
				searchImg := m.copyToSearchBuffer(zoneImg, searchRect)

				// Step=2 (物理步进)
				// ProbeStep=4 (采样步进，即只用 25% 的特征点)
				localX, localY, coarseAvgDiff := MatchProbe(searchImg, m.sharedProbe, 2, 4, true)

				// 追踪阈值
				const trackingMaxDiff = 70.0
				if coarseAvgDiff < trackingMaxDiff {
					finalX := searchRect.Min.X + localX
					finalY := searchRect.Min.Y + localY

					// 快速微调（4px 半径）
					// 修正步进带来的误差。
					fineRadius := 4
					fineROI := image.Rect(
						finalX-fineRadius, finalY-fineRadius,
						finalX+w+fineRadius, finalY+h+fineRadius,
					).Intersect(zoneImg.Bounds())

					finalAvgDiff := coarseAvgDiff

					if fineROI.Dx() >= w {
						fineImg := copySubImage(zoneImg, fineROI)
						// Step=1 (步进)
						// ProbeStep=1 (全采样)
						fx, fy, fineAvgDiffResult := MatchProbe(fineImg, m.sharedProbe, 1, 1, false)

						// Fine search 如果更好，就用 fine 结果
						if fineAvgDiffResult < finalAvgDiff {
							finalX = fineROI.Min.X + fx
							finalY = fineROI.Min.Y + fy
							finalAvgDiff = fineAvgDiffResult
						}
					}

					pos := &MapPosition{
						ZoneID:  m.currentZoneID,
						X:       float64(finalX) + float64(w)/2,
						Y:       float64(finalY) + float64(h)/2,
						AvgDiff: finalAvgDiff,
					}

					// 更新状态并立即返回
					m.updateMotionModel(pos, now)
					m.lastKnownPos = pos
					m.lastTime = now
					m.lostTracking = 0

					return pos, nil
				}
			}
		}
	}

	// ---------------------------------------------------------
	// 全局搜索 (Global Search)
	// ---------------------------------------------------------

	type raceResult struct {
		ZoneID        string
		X, Y          int
		WeightedDiff  float64 // 边缘加权差异
		Consistency   float64 // 局部一致性
		CombinedScore float64 // 最终组合分数
	}

	resultsCh := make(chan raceResult, len(m.zones))
	var wg sync.WaitGroup

	// 优化后的参数：增大步长和采样间隔以提升速度
	// Step=8: 物理步进
	// ProbeStep=8: 采样步进
	coarseStep := 8
	coarseProbeStep := 8

	// 边缘加权参数
	const edgeGamma = 2.0

	// 一致性惩罚权重
	const consistencyAlpha = 0.2

	for zID, zImg := range m.zones {
		wg.Add(1)
		go func(id string, img *image.RGBA) {
			defer wg.Done()

			// 边缘加权匹配（核心算法）
			bx, by, weightedDiff := MatchProbeWeighted(img, m.sharedProbe, coarseStep, coarseProbeStep, false, edgeGamma)

			if weightedDiff == 0 {
				resultsCh <- raceResult{ZoneID: id, WeightedDiff: 0}
				return
			}

			// 快速一致性检查（采样版本，只用1/4的点）
			consistency := ComputeLocalConsistencyFast(img, m.sharedProbe, bx, by, 4)

			// 组合评分
			combinedScore := weightedDiff + consistency*consistencyAlpha

			resultsCh <- raceResult{
				ZoneID:        id,
				X:             bx,
				Y:             by,
				WeightedDiff:  weightedDiff,
				Consistency:   consistency,
				CombinedScore: combinedScore,
			}
		}(zID, zImg)
	}

	wg.Wait()
	close(resultsCh)

	// 收集结果，过滤无效结果（WeightedDiff=0 表示地图太小无法匹配）
	allResults := []raceResult{}
	for res := range resultsCh {
		if res.WeightedDiff > 0 {
			allResults = append(allResults, res)
		}
	}

	// 按 CombinedScore 升序排序（边缘加权 + 一致性惩罚）
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].CombinedScore < allResults[j].CombinedScore
	})

	// 输出详细的三重验证分数
	for i, res := range allResults {
		if i >= 5 {
			break
		}
		log.Debug().
			Int("rank", i+1).
			Str("zone", res.ZoneID).
			Float64("combined", res.CombinedScore).
			Float64("weighted", res.WeightedDiff).
			Float64("consistency", res.Consistency).
			Int("x", res.X).
			Int("y", res.Y).
			Msg("[MapLocate] Global Search Rank")
	}

	// ========================================
	// 三重验证系统 - 置信度判断
	// ========================================
	var winner raceResult
	useWinner := false

	if len(allResults) >= 2 {
		rank1 := allResults[0]
		rank2 := allResults[1]

		// 条件1: 绝对阈值（CombinedScore 足够低）
		// 边缘加权后阈值需要调整，预期正确匹配分数在 28-40 范围
		const maxAbsoluteDiff = 60.0
		absoluteOK := rank1.CombinedScore < maxAbsoluteDiff

		// 条件2: 相对差距（rank1 显著优于 rank2）
		// 边缘加权后差距应该更大，提高阈值
		const minAbsoluteGap = 8.0
		absoluteGap := rank2.CombinedScore - rank1.CombinedScore
		gapOK := absoluteGap >= minAbsoluteGap

		// 条件3: 统计置信度（Z-Score，策略3）
		// rank1 比所有候选的平均值低 1.5 个标准差则认为可信
		var allScores []float64
		for _, r := range allResults {
			allScores = append(allScores, r.CombinedScore)
		}
		zScore := ComputeZScore(rank1.CombinedScore, allScores)
		const minZScore = 1.5
		statOK := zScore > minZScore

		log.Debug().
			Float64("rank1Combined", rank1.CombinedScore).
			Float64("rank2Combined", rank2.CombinedScore).
			Float64("gap", absoluteGap).
			Float64("zScore", zScore).
			Bool("absoluteOK", absoluteOK).
			Bool("gapOK", gapOK).
			Bool("statOK", statOK).
			Msg("[MapLocate] Triple Validation Check")

		// 三重验证：任意两个条件满足即可，避免单一条件导致误判
		conditionsMet := 0
		if absoluteOK {
			conditionsMet++
		}
		if gapOK {
			conditionsMet++
		}
		if statOK {
			conditionsMet++
		}

		if conditionsMet >= 2 {
			winner = rank1
			useWinner = true
		}
	} else if len(allResults) == 1 {
		// 只有一个结果时降级为绝对阈值判断
		if allResults[0].CombinedScore < 55.0 {
			winner = allResults[0]
			useWinner = true
		}
	}

	var bestResult *MapPosition

	if useWinner {
		// Refine Global Winner
		winnerZone := m.zones[winner.ZoneID]
		coarseX, coarseY := winner.X, winner.Y
		fineRadius := 20
		fineROI := image.Rect(
			coarseX-fineRadius, coarseY-fineRadius,
			coarseX+w+fineRadius, coarseY+h+fineRadius,
		).Intersect(winnerZone.Bounds())

		fineImg := copySubImage(winnerZone, fineROI)
		// Fine Search: Step=1, ProbeStep=1
		localX, localY, fineAvgDiff := MatchProbe(fineImg, m.sharedProbe, 1, 1, false)

		// Fine search 不再检查固定阈值
		finalX := fineROI.Min.X + localX
		finalY := fineROI.Min.Y + localY
		cx := float64(finalX) + float64(w)/2
		cy := float64(finalY) + float64(h)/2

		bestResult = &MapPosition{
			ZoneID:  winner.ZoneID,
			X:       cx,
			Y:       cy,
			AvgDiff: fineAvgDiff,
		}

	}

	// Result
	if bestResult != nil {
		// 若切换区域则重置速度。
		if m.currentZoneID != bestResult.ZoneID {
			m.velocityX = 0
			m.velocityY = 0
		} else {
			m.updateMotionModel(bestResult, now)
		}

		m.currentZoneID = bestResult.ZoneID
		m.lastKnownPos = bestResult
		m.lastTime = now
		m.lostTracking = 0

		log.Info().Str("zone", bestResult.ZoneID).
			Float64("x", bestResult.X).
			Float64("y", bestResult.Y).
			Msg("Global Match Resolved")

		return bestResult, nil
	}

	m.lostTracking++
	if m.lostTracking > MaxLostTrackingCount {
		m.lastKnownPos = nil
	}
	return nil, nil
}

func (m *MapLocator) updateMotionModel(newPos *MapPosition, now time.Time) {
	dt := now.Sub(m.lastTime).Seconds()
	if m.lastKnownPos != nil && dt > 0.016 && dt < 1.0 && m.lostTracking == 0 {
		rawVx := (newPos.X - m.lastKnownPos.X) / dt
		rawVy := (newPos.Y - m.lastKnownPos.Y) / dt
		alpha := 0.5
		m.velocityX = m.velocityX*(1-alpha) + rawVx*alpha
		m.velocityY = m.velocityY*(1-alpha) + rawVy*alpha
	} else if m.lostTracking > 0 {
		m.velocityX = 0
		m.velocityY = 0
	}
}

// copyToSearchBuffer 保留复用缓冲区逻辑（与 MapLocator 状态绑定）
func (m *MapLocator) copyToSearchBuffer(src *image.RGBA, r image.Rectangle) *image.RGBA {
	w, h := r.Dx(), r.Dy()
	needed := w * h * 4

	if m.searchBuffer == nil || cap(m.searchBuffer.Pix) < needed {
		m.searchBuffer = image.NewRGBA(image.Rect(0, 0, w, h))
	} else {
		m.searchBuffer.Pix = m.searchBuffer.Pix[:needed]
		m.searchBuffer.Stride = 4 * w
		m.searchBuffer.Rect = image.Rect(0, 0, w, h)
	}

	draw.Draw(m.searchBuffer, m.searchBuffer.Bounds(), src, r.Min, draw.Src)
	return m.searchBuffer
}

func (m *MapLocator) GetLastKnownPos() *MapPosition {
	return m.lastKnownPos
}
