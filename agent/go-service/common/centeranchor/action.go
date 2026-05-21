package centeranchor

import (
	"encoding/json"
	"fmt"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

type clickParams struct {
	// Target is the click target: [x, y] (point) or [x, y, w, h] (rect).
	Target []int `json:"target"`
	// TargetCenter optionally toggles per-axis center anchoring on x / y of Target.
	TargetCenter [2]bool `json:"target_center,omitempty"`
	// TargetOffset is an additive offset, native pipeline semantics.
	TargetOffset *[4]int `json:"target_offset,omitempty"`
	// Contact selects the touch point identifier (ADB: finger index; Win32: mouse button).
	Contact int `json:"contact,omitempty"`
}

type touchMoveParams struct {
	Target       []int   `json:"target"`
	TargetCenter [2]bool `json:"target_center,omitempty"`
	TargetOffset *[4]int `json:"target_offset,omitempty"`
	Pressure     int     `json:"pressure,omitempty"`
	Contact      int     `json:"contact,omitempty"`
}

// ClickAction runs native Click with a center-anchorable target. Param schema
// mirrors native Click with an extra optional target_center: [bool, bool] toggle.
type ClickAction struct{}

var _ maa.CustomActionRunner = (*ClickAction)(nil)

func (a *ClickAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var params clickParams
	if err := json.Unmarshal([]byte(arg.CustomActionParam), &params); err != nil {
		log.Error().Err(err).Str("component", "MpCenterClick").Msg("failed to parse params")
		return false
	}

	imgW, imgH, err := currentImageSize(ctx)
	if err != nil {
		log.Error().Err(err).Str("component", "MpCenterClick").Msg("failed to get current image size")
		return false
	}

	rect, err := resolveTargetRect(params.Target, params.TargetCenter, imgW, imgH)
	if err != nil {
		log.Error().Err(err).
			Str("component", "MpCenterClick").
			Interface("target", params.Target).
			Msg("failed to resolve target")
		return false
	}

	clickParam := maa.ClickParam{
		Target:  maa.NewTargetRect(rect),
		Contact: params.Contact,
	}
	if params.TargetOffset != nil {
		clickParam.TargetOffset = maa.Rect{
			(*params.TargetOffset)[0],
			(*params.TargetOffset)[1],
			(*params.TargetOffset)[2],
			(*params.TargetOffset)[3],
		}
	}

	if _, err := ctx.RunActionDirect(maa.ActionTypeClick, &clickParam, rect, nil); err != nil {
		log.Error().Err(err).Str("component", "MpCenterClick").Msg("inner action failed")
		return false
	}
	return true
}

// TouchMoveAction runs native TouchMove with a center-anchorable target. Param
// schema mirrors native TouchMove with an extra optional target_center.
type TouchMoveAction struct{}

var _ maa.CustomActionRunner = (*TouchMoveAction)(nil)

func (a *TouchMoveAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var params touchMoveParams
	if err := json.Unmarshal([]byte(arg.CustomActionParam), &params); err != nil {
		log.Error().Err(err).Str("component", "MpCenterTouchMove").Msg("failed to parse params")
		return false
	}

	imgW, imgH, err := currentImageSize(ctx)
	if err != nil {
		log.Error().Err(err).Str("component", "MpCenterTouchMove").Msg("failed to get current image size")
		return false
	}

	rect, err := resolveTargetRect(params.Target, params.TargetCenter, imgW, imgH)
	if err != nil {
		log.Error().Err(err).
			Str("component", "MpCenterTouchMove").
			Interface("target", params.Target).
			Msg("failed to resolve target")
		return false
	}

	tmParam := maa.TouchMoveParam{
		Target:   maa.NewTargetRect(rect),
		Pressure: params.Pressure,
		Contact:  params.Contact,
	}
	if params.TargetOffset != nil {
		tmParam.TargetOffset = maa.Rect{
			(*params.TargetOffset)[0],
			(*params.TargetOffset)[1],
			(*params.TargetOffset)[2],
			(*params.TargetOffset)[3],
		}
	}

	if _, err := ctx.RunActionDirect(maa.ActionTypeTouchMove, &tmParam, rect, nil); err != nil {
		log.Error().Err(err).Str("component", "MpCenterTouchMove").Msg("inner action failed")
		return false
	}
	return true
}

// resolveTargetRect handles Target as either a 2-point ([x, y]) or 4-rect spec.
func resolveTargetRect(target []int, center [2]bool, imgW, imgH int) (maa.Rect, error) {
	switch len(target) {
	case 2:
		x, y, err := ResolvePoint([2]int{target[0], target[1]}, center, imgW, imgH)
		if err != nil {
			return maa.Rect{}, err
		}
		return maa.Rect{x, y, 1, 1}, nil
	case 4:
		return ResolveRect([4]int{target[0], target[1], target[2], target[3]}, center, imgW, imgH)
	default:
		return maa.Rect{}, fmt.Errorf("target must be 2 or 4 elements, got %d", len(target))
	}
}

// currentImageSize returns the size of the latest cached screencap.
//
// After P1 startup-time scaling, the cached image is already scaled to the
// 1280x720-anchored reference frame; using GetResolution() here would return
// the unscaled raw size and break center anchoring on non-16:9 devices.
func currentImageSize(ctx *maa.Context) (int, int, error) {
	controller := ctx.GetTasker().GetController()
	if controller == nil {
		return 0, 0, fmt.Errorf("nil controller")
	}
	img, err := controller.CacheImage()
	if err == nil && img != nil {
		bounds := img.Bounds()
		return bounds.Dx(), bounds.Dy(), nil
	}
	// no cached image yet (e.g. action fires before any recognition):
	// force a screencap and read its bounds.
	controller.PostScreencap().Wait()
	img, err = controller.CacheImage()
	if err != nil || img == nil {
		if err == nil {
			err = fmt.Errorf("no cached image after PostScreencap")
		}
		return 0, 0, err
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}
