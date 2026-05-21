package centeranchor

import (
	"encoding/json"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

type templateMatchParams struct {
	ROI       [4]int    `json:"roi"`
	ROICenter [2]bool   `json:"roi_center,omitempty"`
	Template  []string  `json:"template"`
	Threshold []float64 `json:"threshold,omitempty"`
	GreenMask bool      `json:"green_mask,omitempty"`
	OrderBy   string    `json:"order_by,omitempty"`
	Index     int       `json:"index,omitempty"`
	Method    int       `json:"method,omitempty"`
}

type ocrParams struct {
	ROI         [4]int      `json:"roi"`
	ROICenter   [2]bool     `json:"roi_center,omitempty"`
	Expected    []string    `json:"expected,omitempty"`
	Threshold   float64     `json:"threshold,omitempty"`
	Replace     [][2]string `json:"replace,omitempty"`
	OrderBy     string      `json:"order_by,omitempty"`
	Index       int         `json:"index,omitempty"`
	OnlyRec     bool        `json:"only_rec,omitempty"`
	Model       string      `json:"model,omitempty"`
	ColorFilter string      `json:"color_filter,omitempty"`
}

// TemplateMatchRecognition runs native TemplateMatch with a center-anchorable
// ROI. Param schema mirrors native TemplateMatch with an extra optional
// roi_center: [bool, bool] toggle.
type TemplateMatchRecognition struct{}

var _ maa.CustomRecognitionRunner = (*TemplateMatchRecognition)(nil)

func (r *TemplateMatchRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	var params templateMatchParams
	if err := json.Unmarshal([]byte(arg.CustomRecognitionParam), &params); err != nil {
		log.Error().Err(err).Str("component", "MpCenterTemplateMatch").Msg("failed to parse params")
		return nil, false
	}
	bounds := arg.Img.Bounds()
	roi, err := ResolveRect(params.ROI, params.ROICenter, bounds.Dx(), bounds.Dy())
	if err != nil {
		log.Error().Err(err).Str("component", "MpCenterTemplateMatch").Msg("failed to resolve ROI")
		return nil, false
	}
	detail, err := ctx.RunRecognitionDirect(maa.RecognitionTypeTemplateMatch, &maa.TemplateMatchParam{
		ROI:       maa.NewTargetRect(roi),
		Template:  params.Template,
		Threshold: params.Threshold,
		GreenMask: params.GreenMask,
		OrderBy:   maa.TemplateMatchOrderBy(params.OrderBy),
		Index:     params.Index,
		Method:    maa.TemplateMatchMethod(params.Method),
	}, arg.Img)
	if err != nil {
		log.Error().Err(err).Str("component", "MpCenterTemplateMatch").Msg("inner recognition failed")
		return nil, false
	}
	if detail == nil || !detail.Hit {
		return nil, false
	}
	return &maa.CustomRecognitionResult{Box: detail.Box, Detail: detail.DetailJson}, true
}

// OCRRecognition runs native OCR with a center-anchorable ROI. Param schema
// mirrors native OCR with an extra optional roi_center: [bool, bool] toggle.
type OCRRecognition struct{}

var _ maa.CustomRecognitionRunner = (*OCRRecognition)(nil)

func (r *OCRRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	var params ocrParams
	if err := json.Unmarshal([]byte(arg.CustomRecognitionParam), &params); err != nil {
		log.Error().Err(err).Str("component", "MpCenterOCR").Msg("failed to parse params")
		return nil, false
	}
	bounds := arg.Img.Bounds()
	roi, err := ResolveRect(params.ROI, params.ROICenter, bounds.Dx(), bounds.Dy())
	if err != nil {
		log.Error().Err(err).Str("component", "MpCenterOCR").Msg("failed to resolve ROI")
		return nil, false
	}
	detail, err := ctx.RunRecognitionDirect(maa.RecognitionTypeOCR, &maa.OCRParam{
		ROI:         maa.NewTargetRect(roi),
		Expected:    params.Expected,
		Threshold:   params.Threshold,
		Replace:     params.Replace,
		OrderBy:     maa.OCROrderBy(params.OrderBy),
		Index:       params.Index,
		OnlyRec:     params.OnlyRec,
		Model:       params.Model,
		ColorFilter: params.ColorFilter,
	}, arg.Img)
	if err != nil {
		log.Error().Err(err).Str("component", "MpCenterOCR").Msg("inner recognition failed")
		return nil, false
	}
	if detail == nil || !detail.Hit {
		return nil, false
	}
	return &maa.CustomRecognitionResult{Box: detail.Box, Detail: detail.DetailJson}, true
}
