package centeranchor

import (
	"fmt"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// ResolveRect computes the on-screen pixel rect from a partially-anchored spec.
//
// roi[0..3] are interpreted in native MaaFw v5.6+ pipeline semantics: top-left
// coordinates with negative x/y meaning offset from the right/bottom edge.
//
// center toggles per-axis center anchoring:
//   - center[0]=true: roi[0] is treated as a signed offset from imgW/2 instead.
//   - center[1]=true: roi[1] is treated as a signed offset from imgH/2 instead.
//
// roi[2] (width) and roi[3] (height) are always passed through unchanged so
// that MaaFw's native size semantics (0 = extend to edge, negative = abs from
// bottom-right) keep working.
func ResolveRect(roi [4]int, center [2]bool, imgW, imgH int) (maa.Rect, error) {
	if imgW <= 0 || imgH <= 0 {
		return maa.Rect{}, fmt.Errorf("invalid screencap size %dx%d", imgW, imgH)
	}
	x, y := roi[0], roi[1]
	if center[0] {
		x = imgW/2 + roi[0]
	}
	if center[1] {
		y = imgH/2 + roi[1]
	}
	return maa.Rect{x, y, roi[2], roi[3]}, nil
}

// ResolvePoint computes the on-screen pixel point from a partially-anchored
// 2-element spec.
func ResolvePoint(point [2]int, center [2]bool, imgW, imgH int) (int, int, error) {
	if imgW <= 0 || imgH <= 0 {
		return 0, 0, fmt.Errorf("invalid screencap size %dx%d", imgW, imgH)
	}
	x, y := point[0], point[1]
	if center[0] {
		x = imgW/2 + point[0]
	}
	if center[1] {
		y = imgH/2 + point[1]
	}
	return x, y, nil
}
