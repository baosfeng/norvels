package ruleexec

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// pipeValue 是管道里流动的值：字符串，或 split 之后的数组。
type pipeValue struct {
	s    string
	list []string
}

// pipeFunc 是一次管道调用的实现。
type pipeFunc func(ctx *Context, args []string, in pipeValue) (pipeValue, error)

// pipeFuncs 是白名单；不在表里的函数在编译期就被拒绝。
var pipeFuncs = map[string]pipeFunc{
	"replace":   pipeReplace,
	"match":     pipeMatch,
	"split":     pipeSplit,
	"trim":      pipeTrim,
	"first":     pipeFirst,
	"last":      pipeLast,
	"nth":       pipeNth,
	"join":      pipeJoin,
	"concat":    pipeConcat,
	"sub":       pipeSub,
	"urlencode": pipeURLEncode,
	"urldecode": pipeURLDecode,
	"b64enc":    pipeB64Enc,
	"b64dec":    pipeB64Dec,
	"json":      pipeJSON,
	"prefix":    pipePrefix,
	"abs":       pipeAbs,
	"default":   pipeDefault,
	"setvar":    pipeSetVar,
	"getvar":    pipeGetVar,
	"number":    pipeNumber,
	"dedupe":    pipeDedupe,
	"filter":    pipeFilter,
}

// applyPipe 执行一次管道调用。
func applyPipe(ctx *Context, p pipeCall, in pipeValue) (pipeValue, error) {
	fn, ok := pipeFuncs[p.name]
	if !ok {
		return in, fmt.Errorf("ruleexec: 未知管道函数 %q", p.name)
	}
	return fn(ctx, p.args, in)
}

// str 取参数 i（去掉包裹的双引号）。
func str(args []string, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("ruleexec: 管道参数不足（需要第 %d 个）", i+1)
	}
	return strings.Trim(args[i], "\""), nil
}

// re 取参数 i 为正则。
func re(args []string, i int) (*regexp.Regexp, error) {
	raw, err := str(args, i)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "/"), "/")
	return regexp.Compile(raw)
}

func pipeReplace(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	rx, err := re(args, 0)
	if err != nil {
		return in, err
	}
	repl, err := str(args, 1)
	if err != nil {
		return in, err
	}
	in.s = rx.ReplaceAllString(in.s, repl)
	return in, nil
}

func pipeMatch(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	rx, err := re(args, 0)
	if err != nil {
		return in, err
	}
	n := 1
	if len(args) > 1 {
		fmt.Sscanf(args[1], "%d", &n)
	}
	m := rx.FindStringSubmatch(in.s)
	if len(m) == 0 {
		in.s = ""
		return in, nil
	}
	if n < 0 || n >= len(m) {
		n = len(m) - 1
	}
	in.s = m[n]
	return in, nil
}

func pipeSplit(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	sep, err := str(args, 0)
	if err != nil {
		return in, err
	}
	in.list = strings.Split(in.s, sep)
	return in, nil
}

func pipeTrim(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	in.s = strings.TrimSpace(strings.ReplaceAll(in.s, "\u3000", " "))
	return in, nil
}

func pipeFirst(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	return pick(in, 0), nil
}

func pipeLast(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	if len(in.list) == 0 {
		return pipeValue{}, nil
	}
	return pick(in, len(in.list)-1), nil
}

func pipeNth(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	n := 1
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &n)
	}
	return pick(in, n-1), nil
}

// pick 从数组取第 i 个元素（越界返回空）。
func pick(in pipeValue, i int) pipeValue {
	if i < 0 || i >= len(in.list) {
		return pipeValue{}
	}
	return pipeValue{s: in.list[i]}
}

func pipeJoin(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	sep, err := str(args, 0)
	if err != nil {
		return in, err
	}
	in.s = strings.Join(in.list, sep)
	in.list = nil
	return in, nil
}

func pipeConcat(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	s, err := str(args, 0)
	if err != nil {
		return in, err
	}
	in.s += s
	return in, nil
}

func pipeSub(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	from, err := str(args, 0)
	if err != nil {
		return in, err
	}
	to := ""
	if len(args) > 1 {
		to, _ = str(args, 1)
	}
	in.s = substring(in.s, from, to)
	return in, nil
}

// substring 支持负下标（\-1 表示末尾）。
func substring(s, from, to string) string {
	runes := []rune(s)
	n := len(runes)
	start, end := atoiOr(from, 0), n
	if to != "" {
		end = atoiOr(to, n)
	}
	if start < 0 {
		start = n + start
	}
	if end < 0 {
		end = n + end
	}
	start = clamp(start, 0, n)
	end = clamp(end, start, n)
	return string(runes[start:end])
}

func atoiOr(s string, def int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func pipeURLEncode(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	in.s = url.QueryEscape(in.s)
	return in, nil
}

func pipeURLDecode(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	if s, err := url.QueryUnescape(in.s); err == nil {
		in.s = s
	}
	return in, nil
}

func pipeB64Enc(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	in.s = base64.StdEncoding.EncodeToString([]byte(in.s))
	return in, nil
}

func pipeB64Dec(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.s))
	if err != nil || !utf8.Valid(b) {
		in.s = ""
		return in, nil
	}
	in.s = string(b)
	return in, nil
}

func pipeJSON(ctx *Context, args []string, in pipeValue) (pipeValue, error) {
	path, err := str(args, 0)
	if err != nil {
		return in, err
	}
	v, err := parseJSON(in.s)
	if err != nil {
		return pipeValue{}, nil
	}
	s, err := evalJSONPath(v, path)
	if err != nil {
		return in, err
	}
	in.s = s
	return in, nil
}

func pipePrefix(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	base, err := str(args, 0)
	if err != nil {
		return in, err
	}
	in.s = base + in.s
	return in, nil
}

func pipeAbs(ctx *Context, _ []string, in pipeValue) (pipeValue, error) {
	base := ctx.Vars["baseUrl"]
	ri, err := url.Parse(in.s)
	if err != nil || ri.IsAbs() {
		return in, nil
	}
	bi, err := url.Parse(base)
	if err != nil {
		return in, nil
	}
	in.s = bi.ResolveReference(ri).String()
	return in, nil
}

func pipeDefault(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	if in.s != "" {
		return in, nil
	}
	s, err := str(args, 0)
	if err != nil {
		return in, err
	}
	in.s = s
	return in, nil
}

func pipeSetVar(ctx *Context, args []string, in pipeValue) (pipeValue, error) {
	name, err := str(args, 0)
	if err != nil {
		return in, err
	}
	if ctx.Vars == nil {
		ctx.Vars = map[string]string{}
	}
	ctx.Vars[name] = in.s
	return in, nil
}

func pipeGetVar(ctx *Context, args []string, in pipeValue) (pipeValue, error) {
	name, err := str(args, 0)
	if err != nil {
		return in, err
	}
	in.s = ctx.Vars[name]
	return in, nil
}

func pipeNumber(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	n := 0
	fmt.Sscanf(strings.TrimSpace(in.s), "%d", &n)
	in.s = fmt.Sprintf("%d", n)
	return in, nil
}

func pipeDedupe(_ *Context, _ []string, in pipeValue) (pipeValue, error) {
	seen := make(map[string]bool, len(in.list))
	out := make([]string, 0, len(in.list))
	for _, v := range in.list {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	in.list = out
	return in, nil
}

func pipeFilter(_ *Context, args []string, in pipeValue) (pipeValue, error) {
	rx, err := re(args, 0)
	if err != nil {
		return in, err
	}
	out := make([]string, 0, len(in.list))
	for _, v := range in.list {
		if rx.MatchString(v) {
			out = append(out, v)
		}
	}
	in.list = out
	return in, nil
}
