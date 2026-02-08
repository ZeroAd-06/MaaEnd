package AutomaticCharacterTutorial

import (
	"encoding/json"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// DynamicMatchAction presses the key identified by Recognition
type DynamicMatchAction struct{}

// Run implements the custom action logic
func (a *DynamicMatchAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	// 1. Parse Detail from Recognition
	detailStr := arg.RecognitionDetail.DetailJson
	var detail struct {
		Index  int     `json:"index"`
		Score  float64 `json:"score"`
		KeyNum int     `json:"key_num"`
	}

	// Try direct unmarshal first
	if err := json.Unmarshal([]byte(detailStr), &detail); err != nil {
		log.Error().Err(err).Str("detail", detailStr).Msg("Failed to parse recognition detail")
		return false
	}

	// MaaFramework wrapping logic: Check if it's wrapped in "best.detail"
	// If direct unmarshal resulted in default zero values (especially KeyNum which should be non-zero if valid),
	// try to unmarshal the wrapped structure.
	if detail.KeyNum == 0 {
		var wrapped struct {
			Best struct {
				Detail struct {
					Index  int     `json:"index"`
					Score  float64 `json:"score"`
					KeyNum int     `json:"key_num"`
				} `json:"detail"`
			} `json:"best"`
		}
		if err := json.Unmarshal([]byte(detailStr), &wrapped); err == nil {
			// If we successfully found a key_num in the wrapped structure, use it
			if wrapped.Best.Detail.KeyNum != 0 {
				detail = wrapped.Best.Detail
				log.Info().Msg("Parsed wrapped recognition detail successfully")
			}
		}
	}

	// 2. Click the key
	// Logic: "根据序号在key_rois里对应的区域进行ocr识别，然后点击ocr出来的数字键"
	// Recognition has already done the matching and passed `KeyNum`.

	if detail.KeyNum >= 1 && detail.KeyNum <= 4 {
		// Key '1' is ASCII 49 (0x31) in Win32 Virtual Key Codes
		keyCode := 48 + detail.KeyNum
		log.Info().
			Int("keyNum", detail.KeyNum).
			Int("keyCode", keyCode).
			Msg("Pressing skill key (simulating key down/up with delay)")

		ctrl := ctx.GetTasker().GetController()

		// Use KeyDown + Sleep + KeyUp to ensure the game registers the press
		ctrl.PostKeyDown(int32(keyCode)).Wait()

		// Hold the key for 100ms
		time.Sleep(100 * time.Millisecond)

		ctrl.PostKeyUp(int32(keyCode)).Wait()

		return true
	}

	log.Warn().Int("index", detail.Index).Int("keyNum", detail.KeyNum).Msg("No valid key number recognized, skipping action")
	return false
}
