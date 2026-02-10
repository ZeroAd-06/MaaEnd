package keymap

import (
	"encoding/json"
	"time"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

// Default settings
var Win32KeyEnum = map[string]int32{
	// General
	"Move_W":           0x57, // W (Locked)
	"Move_A":           0x41, // A (Locked)
	"Move_S":           0x53, // S (Locked)
	"Move_D":           0x44, // D (Locked)
	"Dash":             0x10, // Shift
	"Jump":             0x20, // Space
	"Interact":         0x46, // F
	"Walk":             0x11, // Ctrl (Locked)
	"Menu":             0x1B, // Esc (Locked)
	"Backpack":         0x42, // B
	"Valuables":        0x4E, // N
	"Team":             0x55, // U
	"Operator":         0x43, // C
	"Mission":          0x4A, // J
	"TrackMission":     0x56, // V
	"Map":              0x4D, // M
	"BackerChat":       0x48, // H
	"Mail":             0x4B, // K
	"Operational":      0x77, // F8
	"Headhunt":         0x78, // F9
	"SwitchModes":      0x09, // Tab (Locked)
	"UseTools":         0x52, // R
	"ExpandToolsWheel": 0x52, // R (LongPress Key) (Followed "UseTools")
	// Combat
	"Attack":              0x01, // Left Mouse Button (Locked)
	"LockToTarget":        0x04, // Middle Mouse Button (Locked)
	"SwitchTarget":        -1,   // Please Use "Scroll" Action. (Locked)
	"CastCombo":           0x45, // E
	"OperatorSkill_1":     0x31, // 1
	"OperatorSkill_2":     0x32, // 2
	"OperatorSkill_3":     0x33, // 3
	"OperatorSkill_4":     0x34, // 4
	"OperatorUltimate_1":  0x31, // 4 (LongPress Key) (Followed "OperatorSkill_1")
	"OperatorUltimate_2":  0x32, // 4 (LongPress Key) (Followed "OperatorSkill_2")
	"OperatorUltimate_3":  0x33, // 4 (LongPress Key) (Followed "OperatorSkill_3")
	"OperatorUltimate_4":  0x34, // 4 (LongPress Key) (Followed "OperatorSkill_4")
	"SwitchOperator_1":    0x70, // F1
	"SwitchOperator_2":    0x71, // F2
	"SwitchOperator_3":    0x72, // F3
	"SwitchOperator_4":    0x73, // F4
	"SwitchOperator_Next": 0x51, // Q
	// AIC Factory
	"AICFactoryPlan":        0x4C, // T
	"TransportBelt":         0x45, // E
	"Pipeline":              0x51, // Q
	"FacilityList":          0x5A, // Z
	"TopViewMode":           0x14, // CapsLock
	"StashMode":             0x58, // X
	"RegionalDeployment":    0x59, // Y
	"Blueprints":            0x70, // F1
	"Show/HideProductIcons": 0x73, // F4
}

// GetKeyCode Transfer key string to key code.
//
// Params:
//   - key: The key string, e.g. "Move_W", "Jump", "Attack", etc.
//
// Returns:
//   - KeyCode(int32): The corresponding key code for the given key string if the key is supported.
//   - -1: If the key string is unsupported.
//   - -2: If the key string is invalid.
func GetKeyCode(key string) int32 {
	var keyCode, ok = Win32KeyEnum[key]
	if !ok {
		log.Error().Msgf("Invalid key: %s", key)
		return -2
	}
	if keyCode == -1 {
		log.Error().Msgf("Unsupported key: %s", key)
	}

	//log.Debug().Msgf("Posting KeyDown: %s (%d | 0x%X)", key, keyCode, keyCode)

	return keyCode
}

// In order to avoid conflict with maafw, add _ for struct names.
type _ClickKey struct{}

type _LongPressKey struct{}

type _KeyDown struct{}

type _KeyUp struct{}

func (a *_ClickKey) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var params struct {
		Key string `json:"key"`
	}

	if err := json.Unmarshal([]byte(arg.CustomActionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse CustomActionParam")
		return false
	}

	key := GetKeyCode(params.Key)
	if key < 0 {
		return false
	}

	return ctx.GetTasker().GetController().PostClickKey(key).Wait().Done()
}

func (a *_LongPressKey) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var params struct {
		Key      string `json:"key"`
		Duration int32  `json:"duration"`
	}

	if err := json.Unmarshal([]byte(arg.CustomActionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse CustomActionParam")
		return false
	}

	key := GetKeyCode(params.Key)
	if key < 0 {
		return false
	}

	var ctrl = ctx.GetTasker().GetController()
	if !ctrl.PostKeyDown(key).Wait().Done() {
		return false
	}

	time.Sleep(time.Duration(params.Duration) * time.Millisecond)

	return ctrl.PostKeyUp(key).Wait().Done()
}

func (a *_KeyDown) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var params struct {
		Key string `json:"key"`
	}

	if err := json.Unmarshal([]byte(arg.CustomActionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse CustomActionParam")
		return false
	}

	key := GetKeyCode(params.Key)
	if key < 0 {
		return false
	}

	return ctx.GetTasker().GetController().PostKeyDown(key).Wait().Done()
}

func (a *_KeyUp) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var params struct {
		Key string `json:"key"`
	}

	if err := json.Unmarshal([]byte(arg.CustomActionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse CustomActionParam")
		return false
	}

	key := GetKeyCode(params.Key)
	if key < 0 {
		return false
	}

	return ctx.GetTasker().GetController().PostKeyUp(key).Wait().Done()
}
