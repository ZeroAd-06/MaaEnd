package centeranchor

import maa "github.com/MaaXYZ/maa-framework-go/v4"

// Register registers MpCenter* custom recognitions and actions.
func Register() {
	maa.AgentServerRegisterCustomRecognition("MpCenterTemplateMatch", &TemplateMatchRecognition{})
	maa.AgentServerRegisterCustomRecognition("MpCenterOCR", &OCRRecognition{})
	maa.AgentServerRegisterCustomAction("MpCenterClick", &ClickAction{})
	maa.AgentServerRegisterCustomAction("MpCenterTouchMove", &TouchMoveAction{})
}
