package parser

import "os"

// writeFile 是测试里的最小写文件助手（避免每个用例都 import os）。
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
