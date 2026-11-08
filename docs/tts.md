# TTS / 朗读（最低依赖）

## 方案

优先使用浏览器 Web Speech API（`window.speechSynthesis` / `SpeechSynthesisUtterance`）：

- 零安装：不新增 npm 依赖，也不把任何二进制打包进仓库。
- 利用系统音源：Chrome/Edge/Safari 会调用操作系统提供的语音引擎。
- 离线可用：系统本地语音不依赖网络；部分浏览器还提供云端音色。

## OS 语音质量差异（调研总结）

- **macOS / iOS**：系统自带“高级语音”（如 Ting-Ting、Ava、Xiaoxiao 对应的本地包），自然度和停顿质量最高；Safari/Chrome 均可 enumerate。部分高级包需要用户在系统设置里下载。
- **Windows**：SAPI5 语音（Microsoft Huihui/HsiaoChen）自然度中等，停顿机械；Edge 另提供在线 Neural 音色，Chrome 走系统 SAPI5。
- **Android**：语音质量高度依赖 OEM/Google TTS 引擎版本；低端设备延迟明显、断句不稳定。
- **Linux**：浏览器多无可用本地语音，`getVoices()` 常为空；建议走外部引擎扩展点。

结论：Web Speech 作为默认零依赖方案；生产级质量优先 macOS/iOS，Windows/Android 可接受，Linux 走外部引擎。

## 控件

朗读控制条（`ReaderTtsBar`）提供：播放/暂停/停止、速度（0.5–2.0）、引擎选择、音色选择、章末自动翻章开关；正在朗读的段落加 `is-tts-active` 高亮。

## 外部轻量 TTS 扩展点

不集成二进制，仅预留接口：`TtsDriver`（`web/src/lib/tts.ts`），任何外部轻量引擎（Piper、Edge-TTS 等）实现该接口并 `registerTtsDriver` 即可出现在引擎下拉里。内置 `external-http` 引擎约定：POST `{ text, rate, voice }`，响应体为音频二进制（`audio/*`），便于用户自托管轻量服务。新引擎的 URL/voice 配置见 ReaderTtsBar 的“外部源地址”。
