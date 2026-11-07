package config

import (
	"strings"
	"testing"
)

// 开放模式（无口令）默认只允许绑回环：绑 0.0.0.0 等于把整库读写开放给同网段，
// 路由器一次端口转发就变成公网可写。要保留内网开放用法必须显式 ALLOW_PUBLIC=1。
func TestCheckOpenListen(t *testing.T) {
	cases := []struct {
		name        string
		addr        string
		token       string
		allowPublic bool
		wantErr     bool
	}{
		{"默认 :8080 无口令拒绝", ":8080", "", false, true},
		{"0.0.0.0 无口令拒绝", "0.0.0.0:8080", "", false, true},
		{"局域网 IP 无口令拒绝", "192.168.1.100:8080", "", false, true},
		{"回环 127.0.0.1 允许", "127.0.0.1:8080", "", false, false},
		{"回环 [::1] 允许", "[::1]:8080", "", false, false},
		{"localhost 允许", "localhost:8080", "", false, false},
		{"无端口回环允许", "127.0.0.1", "", false, false},
		{"有口令允许任意监听", ":8080", "secret", false, false},
		{"显式 ALLOW_PUBLIC 允许", ":8080", "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckOpenListen(tc.addr, tc.token, tc.allowPublic)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("addr=%q token=%q allowPublic=%v：期望拒绝，实际放行", tc.addr, tc.token, tc.allowPublic)
				}
				// 拒绝文案必须给出两条可操作的出路，否则用户只会去搜代码。
				for _, want := range []string{EnvAuthToken, EnvAllowPublic} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("拒绝文案缺少 %s：%v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("addr=%q token=%q allowPublic=%v：期望放行，实际拒绝：%v", tc.addr, tc.token, tc.allowPublic, err)
			}
		})
	}
}
