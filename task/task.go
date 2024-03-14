package task

// 为了能做到每一个环节都可被替换，全部使用接口来处理
type Task interface {
	//最终的业务逻辑
	RunTask()
}

func NewTask(fn func()) Task {
	return &task{fn: fn}
}

type task struct {
	fn func()
}

func (t *task) RunTask() {
	t.fn()
}
