package releases

import (
	"os"
	"strings"
)

// 测试里用到的两个小工具：读仓库文件与切行。
// 放独立文件而不是塞进 builtin_test.go，是为了让断言文件只读断言。

func readRepoFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func splitLines(s string) []string { return strings.Split(s, "\n") }

func trimSpace(s string) string { return strings.TrimSpace(s) }
