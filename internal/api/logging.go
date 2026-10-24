package api

import "log"

// logf 统一走标准库 log（测试可替换 s.log 静音或捕获输出）。
func (s *Server) logf(format string, args ...any) {
	if s.log != nil {
		s.log.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}
