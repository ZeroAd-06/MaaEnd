# 开发手册 - CenterAnchor 参考文档

`CenterAnchor` 是一组 go-service 提供的自定义组件，包装原生识别 / 动作并加入"按屏幕中心对齐 ROI / Target"的能力，用于跨非 1280×720 窗口时仍能正确命中。

包含 4 个组件，每个都对应一个原生节点：

| 自定义名                  | 包装的原生节点                                | 参数键               |
| ------------------------- | --------------------------------------------- | -------------------- |
| `MpCenterTemplateMatch`   | `recognition: "TemplateMatch"`                | `roi_center: [bool, bool]` |
| `MpCenterOCR`             | `recognition: "OCR"`                          | `roi_center: [bool, bool]` |
| `MpCenterClick`           | `action: "Click"`                             | `target_center: [bool, bool]` |
| `MpCenterTouchMove`       | `action: "TouchMove"`                         | `target_center: [bool, bool]` |

当前实现位于：

- Go 包：`agent/go-service/common/centeranchor/`
  - `resolve.go` — 核心 `ResolveRect` / `ResolvePoint` 计算
  - `recognition.go` — `TemplateMatchRecognition` / `OCRRecognition`
  - `action.go` — `ClickAction` / `TouchMoveAction`
  - `register.go` — 包内注册入口
- go-service 总注册入口：`agent/go-service/register.go`

## 它解决的问题

MaaEnd 的所有 Pipeline ROI / 坐标都以 1280×720 为参考。MaaFramework 自身在截图阶段把宽边或短边缩放到这个参考帧（参见 `agent/go-service/taskersink/aspectratio/checker.go` 中的 `chooseScreenshotOption`）：

- **16:9 窗口**：长边 = 1280，按 1280×720 直接对齐。
- **更宽窗口（如 21:9）**：短边 = 720，长边超过 1280，需要从中心或右边缘锚定。
- **更窄窗口（如 4:3）**：长边 = 1280，短边小于 720，仍然左上对齐，但下方区域被裁。

对于左上贴边的 UI（绝大多数 HUD），coords 保持原样即可命中；对于右 / 下贴边的 UI，MaaFramework v5.6+ 允许 ROI / target 写负坐标实现右下边缘锚定。**只有屏幕中心附近的 UI（菜单弹窗、对话框、设置面板等）需要这套组件**，因为它们既不贴左上也不贴右下。

## 调用方式

### 中心锚点识别

把原节点的 `recognition` 改成自定义形式即可。`roi_center` 是一个 `[bool, bool]` 数组，分别指示 x、y 轴是否启用中心锚定：

```jsonc
"MyCenterCheck": {
    "recognition": "Custom",
    "custom_recognition": "MpCenterOCR",
    "custom_recognition_param": {
        "roi": [-220, 260, 450, 100],
        "roi_center": [true, true],
        "expected": ["选项"]
    }
}
```

- `roi_center[0] = true` 表示 `roi[0]` 是相对屏幕中心 x 的偏移；
- `roi_center[1] = true` 表示 `roi[1]` 是相对屏幕中心 y 的偏移；
- `roi[2]` / `roi[3]` 是宽高，保持不变。

模板匹配几乎一致：

```jsonc
"MyCenterTemplate": {
    "recognition": "Custom",
    "custom_recognition": "MpCenterTemplateMatch",
    "custom_recognition_param": {
        "roi": [-200, -150, 400, 300],
        "roi_center": [true, true],
        "template": "MyFeature/Button.png",
        "threshold": 0.85
    }
}
```

参数字段（`template` / `threshold` / `expected` 等）会原样传给原生节点，因此其它字段含义和官方 Pipeline 协议一致。

### 中心锚点动作

`target` 支持 `[x, y]`（点）或 `[x, y, w, h]`（矩形）两种写法。`target_center` 同样是 `[bool, bool]`：

```jsonc
"MyCenterClick": {
    "action": "Custom",
    "custom_action": "MpCenterClick",
    "custom_action_param": {
        "target": [-100, -50, 200, 100],
        "target_center": [true, true]
    }
}
```

`target_offset`、`contact`、`pressure` 等原生字段继续可用，含义与原生一致。

## 何时不需要用

只有锚点类型与原始 Pipeline 不一致时才改。最常见的两类**不需要**迁移：

- **左上贴边的 UI**：例如左上角的菜单按钮，正坐标即可命中。
- **右 / 下贴边的 UI**：把对应坐标改成负数即可，MaaFramework v5.6+ 直接支持，无需自定义组件。

```jsonc
"RightEdgeButton": {
    "recognition": "OCR",
    "roi": [-200, 80, 180, 50],   // 右上角
    "expected": ["继续"]
}
```

## 实现要点

- `currentImageSize` 通过 `controller.CacheImage()` 取当前缓存截图的尺寸——这个尺寸是 P1 启动时缩放后的"逻辑参考帧"大小。**不要**改用 `controller.GetResolution()`，那个返回的是设备原始分辨率，会破坏锚点。
- `resolveTargetRect` 同时处理 2 元 / 4 元 `target`；2 元会被补成 `[x, y, 1, 1]`，与原生 Click 行为一致。
- 注册时机：`agent/go-service/register.go` 在启动期调用 `centeranchor.Register()`，必须早于任何任务执行。

## 自检清单

接入时检查：

1. ROI / target 是否真的需要按中心对齐？多数 HUD 不需要。
2. `roi_center` / `target_center` 的两位 bool 是否正确——上下贴中心常常只勾 y 轴，左右贴中心常常只勾 x 轴。
3. 在 16:9 窗口下，中心锚点节点的 ROI 是否与原 1280×720 ROI 等价（中心点偏移 + 宽高不变）。
4. 在 21:9 或 4:3 窗口下手动跑一次任务，确认目标 UI 命中。

## 相关文档

- [Custom 动作与识别参考文档](../custom.md)：了解 `Custom` 动作与识别的通用调用方式。
- [SceneManager 场景跳转](../scene-manager.md)：万能跳转里的 Mp 节点大多是这套组件的真实用例。
- [编码规范](../coding-standards.md)：了解 Pipeline / Go Service 的整体开发规范。
