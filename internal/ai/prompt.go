package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 提示词与体量上限：单段截断到 1000 rune、上下文 400 rune、最多 50 处，
// 既控制成本，也限制一次外发的正文体量。
const (
	maxSegments     = 50
	maxSegmentRunes = 1000
	maxContextRunes = 400
)

// systemPrompt 要求模型只吐一个 JSON 对象，并禁止用「A」「B」写理由——
// 呈现顺序每次调用都随机，理由里带标签会与映射后的结论对不上。
const systemPrompt = `你是中文小说正文的版本裁决器。
用户给出若干处差异位置的两版正文：每处都标为「版本 A」与「版本 B」（哪个是旧版由程序随机决定，与质量无关），并附带该处上下文。
请整体判断哪一版更好：错字错句更少、乱码与广告噪音更少、句子更通顺、内容更完整。
只输出一个 JSON 对象，不要代码块、不要任何解释文字：
{"winner":"A|B|equal","confidence":0..1,"reasons":["..."]}
- winner：取 "A"、"B" 或 "equal"（两版无实质差别）；
- confidence：0 到 1 之间的数，表示你对这个判断的把握；
- reasons：1 到 3 条简短依据（中文，每条不超过 40 字），不要用「A」「B」指代，直接说依据。`

// clampSegments 丢掉 A、B 都为空的段落，并按 maxSegments 截断（抽样由调用方负责）。
func clampSegments(segs []Segment) []Segment {
	out := make([]Segment, 0, len(segs))
	for _, s := range segs {
		if strings.TrimSpace(s.A) == "" && strings.TrimSpace(s.B) == "" {
			continue
		}
		out = append(out, s)
		if len(out) == maxSegments {
			break
		}
	}
	return out
}

// renderSegments 把段落渲染成编号正文。swapped 为 true 时把 Segment.B 放进「版本 A」，
// 即标签不动、内容互换——这是每次调用随机化的落点，解析后再按同一开关映射回真实 A/B。
func renderSegments(segs []Segment, swapped bool) string {
	var b strings.Builder
	for i, s := range segs {
		fmt.Fprintf(&b, "第 %d 处\n", i+1)
		if ctx := truncateRunes(s.Context, maxContextRunes); ctx != "" {
			fmt.Fprintf(&b, "上下文：%s\n", ctx)
		}
		first, second := s.A, s.B
		if swapped {
			first, second = s.B, s.A
		}
		fmt.Fprintf(&b, "版本 A：%s\n版本 B：%s\n\n", truncateRunes(first, maxSegmentRunes), truncateRunes(second, maxSegmentRunes))
	}
	return strings.TrimRight(b.String(), "\n")
}

// truncateRunes 去空白并按 rune 截断（不劈开多字节字符）；截断时补省略号。
func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// verdictPayload 是模型该吐的 JSON 形态。
type verdictPayload struct {
	Winner     string   `json:"winner"`
	Confidence float64  `json:"confidence"`
	Reasons    []string `json:"reasons"`
}

// parseVerdict 解析模型给出的内容；宽容代码块围栏，其余不符合契约一律报 ErrBadResponse。
func parseVerdict(content string) (Verdict, error) {
	raw := stripFence(content)
	if raw == "" {
		return Verdict{}, fmt.Errorf("%w：响应为空", ErrBadResponse)
	}
	var p verdictPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Verdict{}, fmt.Errorf("%w：%v", ErrBadResponse, err)
	}
	return p.normalize()
}

// normalize 校验并归一化字段：winner 大小写不敏感，confidence 必须落在 0..1。
func (p verdictPayload) normalize() (Verdict, error) {
	winner, err := normalizeWinner(p.Winner)
	if err != nil {
		return Verdict{}, err
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		return Verdict{}, fmt.Errorf("%w：confidence %v 超出 0..1", ErrBadResponse, p.Confidence)
	}
	reasons := make([]string, 0, len(p.Reasons))
	for _, r := range p.Reasons {
		if r = strings.TrimSpace(r); r != "" {
			reasons = append(reasons, r)
		}
	}
	return Verdict{Winner: winner, Confidence: p.Confidence, Reasons: reasons}, nil
}

// normalizeWinner 把模型的 winner 归到三个常量之一。
func normalizeWinner(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a":
		return WinnerA, nil
	case "b":
		return WinnerB, nil
	case "equal":
		return WinnerEqual, nil
	default:
		return "", fmt.Errorf("%w：winner %q 不是 A/B/equal", ErrBadResponse, s)
	}
}

// stripFence 去掉 markdown 代码块围栏（模型偶尔会包一层 json 围栏）。
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

// mapWinner 把「模型看到的 A/B」映射回调用方真实的 A/B；equal 与理由不受影响。
// 理由由提示词约束为不带 A/B 指代，故只映射 winner。
func mapWinner(v Verdict, swapped bool) Verdict {
	if !swapped || v.Winner == WinnerEqual {
		return v
	}
	if v.Winner == WinnerA {
		v.Winner = WinnerB
	} else if v.Winner == WinnerB {
		v.Winner = WinnerA
	}
	return v
}
