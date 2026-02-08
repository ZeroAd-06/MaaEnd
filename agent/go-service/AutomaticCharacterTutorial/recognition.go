package AutomaticCharacterTutorial

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// DynamicMatchRecognition implements logic to match a skill icon and recognize the corresponding key
type DynamicMatchRecognition struct{}

// Run implements the custom recognition logic
func (r *DynamicMatchRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	// 1. Define parameters (Hardcoded as per request)
	params := struct {
		TopROI     []int
		SkillROI   []int
		BottomROIs [][]int
		KeyROIs    [][]int
		Threshold  float64
	}{
		TopROI: []int{617, 49, 45, 66},
		// SkillROI: Modified from {626, 57, 28, 28} to {629, 60, 22, 22}
		// Reason: Crop the center 22x22 pixels to remove edge glow/background noise.
		// The top icon has a different glowing background than the bottom icon.
		// Using the full 28x28 area includes this noise, causing false positives (higher scores on wrong icons).
		// A smaller, cleaner center crop improves the signal-to-noise ratio for matching.
		SkillROI: []int{629, 60, 22, 22},
		BottomROIs: [][]int{
			// Expanded ROIs to ensure template fits and allows matching movement
			// Reduced size from 60x60 to 40x40 to reduce background noise influence
			// Centers kept roughly same:
			// 0: Center (1244, 643) -> {1224, 623, 40, 40}
			{1224, 623, 40, 40},
			// 1: Center (1180, 643) -> {1160, 623, 40, 40}
			{1160, 623, 40, 40},
			// 2: Center (1117, 644) -> {1097, 624, 40, 40}
			{1097, 624, 40, 40},
			// 3: Center (1054, 644) -> {1034, 624, 40, 40}
			{1034, 624, 40, 40},
		},
		KeyROIs: [][]int{
			{1233, 670, 20, 20},
			{1169, 670, 20, 20},
			{1105, 670, 20, 20},
			{1041, 670, 21, 20},
		},
		Threshold: 0.25, // Updated based on user feedback
	}

	// Default threshold
	if params.Threshold == 0 {
		params.Threshold = 0.7
	}
	// If user provided > 1.0 (e.g. 60 or 80), normalize it to 0.0-1.0
	if params.Threshold > 1.0 {
		params.Threshold = params.Threshold / 100.0
	}

	img := arg.Img
	if img == nil {
		return nil, false
	}

	// Helper interface for cropping
	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	subImager, ok := img.(SubImager)
	if !ok {
		log.Error().Msg("Image does not support SubImage")
		return nil, false
	}

	// Helper: Binarize image for Template (White Icon, Green Background)
	// Threshold: 100 (out of 255) - Lowered to capture more details
	binarizeTemplate := func(src image.Image) image.Image {
		bounds := src.Bounds()
		dst := image.NewRGBA(bounds)
		threshold := uint32(25700) // 100/255 * 65535 ≈ 25700
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := src.At(x, y).RGBA()
				if r > threshold && g > threshold && b > threshold {
					dst.Set(x, y, color.White)
				} else {
					dst.Set(x, y, color.Black)
				}
			}
		}
		return dst
	}

	// Helper: Binarize image for Search (White Icon, Black Background)
	binarizeSearch := func(src image.Image) image.Image {
		bounds := src.Bounds()
		dst := image.NewRGBA(bounds)
		threshold := uint32(25700) // 100/255 * 65535 ≈ 25700
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := src.At(x, y).RGBA()
				if r > threshold && g > threshold && b > threshold {
					dst.Set(x, y, color.White)
				} else {
					dst.Set(x, y, color.Black)
				}
			}
		}
		return dst
	}

	// 2. Prepare Template (SkillROI first, then TopROI)
	createTempTemplate := func(roi []int) (string, string, error) {
		if len(roi) < 4 {
			return "", "", os.ErrInvalid
		}
		rect := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3])
		if !rect.In(img.Bounds()) {
			rect = rect.Intersect(img.Bounds())
		}
		if rect.Empty() {
			return "", "", os.ErrInvalid
		}

		cropImg := subImager.SubImage(rect)
		binarizedImg := binarizeTemplate(cropImg) // Use Template Binarization

		relDir := "AutomaticCharacterTutorial"
		// Use unique filename to avoid caching issues
		fileName := fmt.Sprintf("dynamic_template_%d.png", time.Now().UnixNano())
		relPath := relDir + "/" + fileName
		absDir := filepath.Join("resource", "image", relDir)
		if err := os.MkdirAll(absDir, 0755); err != nil {
			return "", "", err
		}
		fullPath := filepath.Join(absDir, fileName)
		f, err := os.Create(fullPath)
		if err != nil {
			return "", "", err
		}
		defer f.Close()
		if err := png.Encode(f, binarizedImg); err != nil {
			return "", "", err
		}
		return relPath, fullPath, nil
	}

	log.Debug().Ints("SkillROI", params.SkillROI).Msg("Capturing template from SkillROI")
	templatePath, fullTemplatePath, err := createTempTemplate(params.SkillROI)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create template from SkillROI, trying TopROI")
		templatePath, fullTemplatePath, err = createTempTemplate(params.TopROI)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create template from both SkillROI and TopROI")
			return nil, false
		}
	}
	// Clean up the template file after recognition is done
	defer os.Remove(fullTemplatePath)

	// 3. Match Template against BottomROIs
	// Pre-process the search image to match the template style (White on Black)
	searchImg := binarizeSearch(img)

	bestIdx := -1
	maxScore := -1.0

	for i, bottomROI := range params.BottomROIs {
		if len(bottomROI) < 4 {
			continue
		}

		taskName := "DynamicMatch_" + strconv.Itoa(i)
		tmParam := map[string]any{
			taskName: map[string]any{
				"recognition": "TemplateMatch",
				"template":    templatePath,
				"threshold":   params.Threshold,
				"roi":         bottomROI,
				"method":      5, // TM_CCOEFF_NORMED
			},
		}

		// Use the pre-processed search image (White on Black)
		res := ctx.RunRecognition(taskName, searchImg, tmParam)

		var score float64
		if res != nil {
			var detail struct {
				All []struct {
					Score float64 `json:"score"`
				} `json:"all"`
				Best *struct {
					Score float64 `json:"score"`
				} `json:"best"`
			}
			// Best effort to parse score
			if err := json.Unmarshal([]byte(res.DetailJson), &detail); err == nil {
				if detail.Best != nil {
					score = detail.Best.Score
				} else if len(detail.All) > 0 {
					// Fallback: find max score in 'all'
					for _, item := range detail.All {
						if item.Score > score {
							score = item.Score
						}
					}
				}
			}
		} else {
			// Not hit, score is effectively 0 or low
			score = 0.0
		}

		log.Debug().Int("index", i).Float64("score", score).Msg("Template match result")

		if score > maxScore {
			maxScore = score
			bestIdx = i
		}
	}

	// Check if the best match is good enough
	if maxScore < params.Threshold {
		log.Info().Float64("maxScore", maxScore).Msg("No matching skill icon found (score too low)")
		return nil, false
	}

	log.Info().Int("bestIdx", bestIdx).Float64("score", maxScore).Msg("Skill matched")

	// 4. Identify Key Number using Template Match (1.png - 4.png)
	// Only perform OCR on the KeyROI corresponding to the matched BottomROI
	keyNum := -1
	if bestIdx >= 0 && bestIdx < len(params.KeyROIs) {
		// Get the KeyROI for the matched position
		baseKeyROI := params.KeyROIs[bestIdx]

		// Expand ROI slightly to ensure template fits and allows for slight offset
		searchROI := []int{
			baseKeyROI[0] - 10,
			baseKeyROI[1] - 10,
			baseKeyROI[2] + 20,
			baseKeyROI[3] + 20,
		}

		bestKeyScore := -1.0

		for k := 1; k <= 4; k++ {
			templateName := "AutomaticCharacterTutorial/" + strconv.Itoa(k) + ".png"
			taskName := "MatchKey_" + strconv.Itoa(k)

			tmParam := map[string]any{
				taskName: map[string]any{
					"recognition": "TemplateMatch",
					"template":    templateName,
					"threshold":   0.6, // Lower threshold for small icons
					"roi":         searchROI,
					"method":      5,
				},
			}

			res := ctx.RunRecognition(taskName, img, tmParam)
			if res != nil {
				var detail struct {
					Best *struct {
						Score float64 `json:"score"`
					} `json:"best"`
					All []struct {
						Score float64 `json:"score"`
					} `json:"all"`
				}

				score := 0.0
				if err := json.Unmarshal([]byte(res.DetailJson), &detail); err == nil {
					if detail.Best != nil {
						score = detail.Best.Score
					} else if len(detail.All) > 0 {
						for _, item := range detail.All {
							if item.Score > score {
								score = item.Score
							}
						}
					}
				}

				log.Debug().Int("key", k).Float64("score", score).Msg("Key number match result")

				if score > bestKeyScore {
					bestKeyScore = score
					keyNum = k
				}
			}
		}

		// If score is too low, maybe it's not a valid number?
		// But we should return the best guess if it's reasonable.
		if bestKeyScore < 0.5 {
			log.Warn().Float64("score", bestKeyScore).Msg("Key number match score too low")
			// keyNum = -1 // Optional: strict check
		}
	}

	// 5. Return Result
	// Pass the bestIdx and recognized keyNum to Action
	// Note: We return Hit=true even if OCR failed, because we found the skill icon.
	// Action will decide what to do if keyNum is missing (though instructions say "click ocr'd key").
	// If OCR failed, keyNum will be -1.

	detailBytes, _ := json.Marshal(map[string]any{
		"index":   bestIdx,
		"score":   maxScore,
		"key_num": keyNum,
	})

	// Box can be the matched BottomROI for visualization
	box := maa.Rect{}
	if bestIdx >= 0 && bestIdx < len(params.BottomROIs) {
		r := params.BottomROIs[bestIdx]
		if len(r) >= 4 {
			box = maa.Rect{r[0], r[1], r[2], r[3]}
		}
	}

	return &maa.CustomRecognitionResult{
		Box:    box,
		Detail: string(detailBytes),
	}, true
}
