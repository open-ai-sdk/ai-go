package uistream

import "fmt"

func recoverPanic(onPanic func(error)) {
	value := recover()
	if value == nil || onPanic == nil {
		return
	}
	if err, ok := value.(error); ok {
		onPanic(err)
		return
	}
	onPanic(fmt.Errorf("panic: %v", value))
}

func safeObserver(observer func()) {
	if observer == nil {
		return
	}
	defer recoverPanic(nil)
	observer()
}
