package debug

import (
	"fmt"
	"path/filepath"
	"runtime"
)

func prefix(step int) string {
	_, file, line, ok := runtime.Caller(2 + step)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d: ", filepath.Base(file), line)
}

func SetLogger(fn func(...interface{})) {
	logfunc = fn
}

func SetLoggerf(fn func(string, ...interface{})) {
	if fn != nil {
		logfunc = func(args ...interface{}) {
			fn("%s", fmt.Sprint(args...))
		}
	} else {
		logfunc = nil
	}
}

func Logf(format string, args ...interface{}) {
	if logfunc == nil {
		return
	}
	logfunc(prefix(1), fmt.Sprintf(format, args...))
}

func Log(args ...interface{}) {
	if logfunc == nil {
		return
	}
	logfunc(append(append(make([]interface{}, 0, len(args)+1), prefix(1)), args...)...)
}

// logfunc should follow fmt.Sprint formatting rules
var logfunc = func(...interface{}) {}
