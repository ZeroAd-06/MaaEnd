package AutomaticCharacterTutorial

import (
	"encoding/json"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// UltimateSkillAction implements logic to long press the ultimate key
// 终结技动作：根据识别到的按键数字长按对应的键盘按键
type UltimateSkillAction struct{}

// Run implements the custom action logic
func (a *UltimateSkillAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	// 1. Get detail
	detailStr := arg.RecognitionDetail.DetailJson
	var detail struct {
		Index  int `json:"index"`
		KeyNum int `json:"key_num"`
	}

	// Try direct unmarshal first
	if err := json.Unmarshal([]byte(detailStr), &detail); err != nil {
		log.Error().Err(err).Str("detail", detailStr).Msg("Failed to parse ult action detail")
		return false
	}

	// Wrapper handling (just in case)
	if detail.KeyNum == 0 {
		var wrapped struct {
			Best struct {
				Detail struct {
					Index  int `json:"index"`
					KeyNum int `json:"key_num"`
				} `json:"detail"`
			} `json:"best"`
		}
		if err := json.Unmarshal([]byte(detailStr), &wrapped); err == nil {
			if wrapped.Best.Detail.KeyNum != 0 {
				detail = wrapped.Best.Detail
			}
		}
	}

	if detail.KeyNum >= 1 && detail.KeyNum <= 4 {
		keyCode := 48 + detail.KeyNum
		log.Info().Int("keyNum", detail.KeyNum).Int("keyCode", keyCode).Msg("Long pressing ultimate skill key (0.3s)")

		ctrl := ctx.GetTasker().GetController()

		// Press Down
		ctrl.PostKeyDown(int32(keyCode)).Wait()

		// Hold for 300ms
		time.Sleep(300 * time.Millisecond)

		// Release
		ctrl.PostKeyUp(int32(keyCode)).Wait()

		return true
	}

	log.Warn().Int("index", detail.Index).Int("keyNum", detail.KeyNum).Msg("No valid key number for ult, skipping action")
	return false
}
