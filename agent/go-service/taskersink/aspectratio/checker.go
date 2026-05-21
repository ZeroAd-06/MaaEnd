package aspectratio

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/control"
	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/i18n"
	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/maafocus"
	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/pienv"
	"github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

const (
	targetRatio  = 16.0 / 9.0
	tolerance    = 0.02
	targetWidth  = 1280
	targetHeight = 720
)

// AspectRatioChecker configures screenshot scaling once before any pipeline
// runs, so that pipelines designed against the 1280x720 reference work across
// arbitrary landscape aspect ratios.
//
// 思路与等价于 MaaFramework 上游 #1336 的 ScreenshotTargetExpand 模式（仅限
// 横屏场景）：用 SetScreenshot 的 LongSide / ShortSide 把短边或长边锁到参考
// 尺寸，截图按等比缩放后，UI 元素的像素尺寸与 1280x720 设计稿一致。
type AspectRatioChecker struct {
	once sync.Once
}

// OnTaskerTask handles tasker task events.
func (c *AspectRatioChecker) OnTaskerTask(tasker *maa.Tasker, event maa.EventStatus, detail maa.TaskerTaskDetail) {
	if event != maa.EventStatusStarting {
		return
	}
	if detail.Entry == "MaaTaskerPostStop" {
		log.Debug().Msg("Received PostStop event, skipping screenshot scaling setup")
		return
	}
	c.once.Do(func() {
		c.setupScreenshotScaling(tasker, detail)
	})
}

func (c *AspectRatioChecker) setupScreenshotScaling(tasker *maa.Tasker, detail maa.TaskerTaskDetail) {
	controller := tasker.GetController()
	if controller == nil {
		log.Error().Uint64("task_id", detail.TaskID).Msg("Failed to get controller from tasker")
		return
	}

	width, height, ok := readResolutionWithRetry(controller)
	if !ok {
		log.Error().
			Uint64("task_id", detail.TaskID).
			Int32("width", width).
			Int32("height", height).
			Msg("Resolution still too small after max retries; skipping screenshot scaling setup")
		return
	}

	controlType, controllerTypeSource, controlErr := resolveControllerType(controller)
	controllerDisplay := displayController(pienv.ControllerName(), controlType)
	if controlErr != nil {
		log.Warn().
			Err(controlErr).
			Uint64("task_id", detail.TaskID).
			Str("entry", detail.Entry).
			Str("controller_name", pienv.ControllerName()).
			Str("controller_type_from_pi", pienv.ControllerType()).
			Int32("width", width).
			Int32("height", height).
			Msg("Failed to detect controller type")
	}

	if width <= height {
		log.Error().
			Uint64("task_id", detail.TaskID).
			Str("entry", detail.Entry).
			Str("controller_type", controlType).
			Str("controller_type_source", controllerTypeSource).
			Int32("width", width).
			Int32("height", height).
			Msg("Portrait resolution not supported; stopping task")
		c.stopWithWarning(tasker, controllerDisplay, int(width), int(height),
			i18n.T("tasker.aspect_ratio_warning.portrait_unsupported"))
		return
	}

	opt, mode := chooseScreenshotOption(int(width), int(height))
	if err := controller.SetScreenshot(opt); err != nil {
		log.Error().
			Err(err).
			Uint64("task_id", detail.TaskID).
			Str("mode", mode).
			Int32("width", width).
			Int32("height", height).
			Msg("Failed to apply screenshot scaling")
		c.stopWithWarning(tasker, controllerDisplay, int(width), int(height),
			i18n.T("tasker.aspect_ratio_warning.scaling_failed"))
		return
	}

	log.Info().
		Uint64("task_id", detail.TaskID).
		Str("entry", detail.Entry).
		Str("controller_name", pienv.ControllerName()).
		Str("controller_type", controlType).
		Str("controller_type_source", controllerTypeSource).
		Int32("width", width).
		Int32("height", height).
		Str("mode", mode).
		Msg("Configured screenshot scaling")
}

// chooseScreenshotOption picks a screenshot-scaling option that makes the
// 1280x720 reference layout work for any landscape aspect ratio.
//
// Caller must ensure width > height (landscape).
func chooseScreenshotOption(width, height int) (maa.ScreenshotOption, string) {
	ratio := float64(width) / float64(height)
	switch {
	case math.Abs(ratio-targetRatio) <= targetRatio*tolerance:
		return maa.WithScreenshotTargetLongSide(targetWidth), "16:9"
	case ratio > targetRatio:
		return maa.WithScreenshotTargetShortSide(targetHeight), "wider_than_16x9"
	default:
		return maa.WithScreenshotTargetLongSide(targetWidth), "narrower_than_16x9"
	}
}

func readResolutionWithRetry(controller *maa.Controller) (int32, int32, bool) {
	const maxRetries = 20
	var width, height int32
	var err error
	for i := range maxRetries {
		width, height, err = controller.GetResolution()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get resolution")
			return width, height, false
		}
		if width > 100 && height > 100 {
			return width, height, true
		}
		log.Debug().
			Int32("width", width).
			Int32("height", height).
			Int("attempt", i+1).
			Msg("Resolution too small, window may not be ready yet, retrying...")
		time.Sleep(time.Second)
		controller.PostScreencap().Wait()
	}
	return width, height, false
}

func (c *AspectRatioChecker) stopWithWarning(tasker *maa.Tasker, controllerDisplay string, width, height int, followUpLines ...string) {
	maafocus.PrintLargeContentTrimNewline(
		i18n.RenderHTML("tasker.aspect_ratio_warning", buildWarningData(controllerDisplay, width, height, followUpLines...)),
	)
	tasker.PostStop()
}

func resolveControllerType(controller *maa.Controller) (string, string, error) {
	if controlType := normalizeControllerType(pienv.ControllerType()); controlType != "" {
		return controlType, "pi_env", nil
	}
	controlType, err := control.GetControlType(controller)
	if err != nil {
		return "unknown", "controller_info", err
	}
	if normalized := normalizeControllerType(controlType); normalized != "" {
		return normalized, "controller_info", nil
	}
	return "unknown", "controller_info", nil
}

func buildWarningData(controllerDisplay string, width, height int, followUpLines ...string) map[string]any {
	lines := make([]string, 0, len(followUpLines))
	for _, line := range followUpLines {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return map[string]any{
		"ControllerType":    controllerDisplay,
		"CurrentResolution": fmt.Sprintf("%dx%d", width, height),
		"FollowUpLines":     lines,
	}
}

func displayController(name, controllerType string) string {
	typeLabel := displayControllerType(controllerType)
	if name == "" {
		if typeLabel == "" {
			return "unknown"
		}
		return typeLabel
	}
	if typeLabel == "" || strings.EqualFold(name, typeLabel) {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, typeLabel)
}

func displayControllerType(controllerType string) string {
	switch controllerType {
	case control.CONTROL_TYPE_ADB:
		return "ADB"
	case control.CONTROL_TYPE_WIN32:
		return "Win32"
	case control.CONTROL_TYPE_WLROOTS:
		return "Wlroots"
	default:
		return controllerType
	}
}

func normalizeControllerType(controllerType string) string {
	switch strings.ToLower(strings.TrimSpace(controllerType)) {
	case "adb":
		return control.CONTROL_TYPE_ADB
	case "win32":
		return control.CONTROL_TYPE_WIN32
	case "wlroots":
		return control.CONTROL_TYPE_WLROOTS
	default:
		return ""
	}
}
