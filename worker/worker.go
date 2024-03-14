package worker

import (
	"github.com/Diana-Fox/CoroutinePool/task"
	"sync/atomic"
)

type Worker interface {
	RunWork()               //启动协程
	SetWork(task task.Task) //方便回收调度
	GetRecyclingCode() string
	OverWorker() //结束协程
}

const (
	Acceptable = iota + 1 //可以接收任务
	Ready                 //就绪态
	Running               //可以被执行
	Execute               //执行态
)

type CoroutineWorker struct {
	task         task.Task     //要有个任务
	taskStatus   atomic.Uint32 //任务的状态
	workerStatus atomic.Bool   //任务协程的生死
	signal       chan string   //方便任务完成的时候，归入空闲协程
	//one           sync.Once     //控制只能启动一次协程
	recyclingCode string //回收码
}

func (c *CoroutineWorker) GetRecyclingCode() string {
	return c.recyclingCode
}

func NewWorker(signal chan string, recyclingCode string) Worker {
	return &CoroutineWorker{
		signal:        signal,
		recyclingCode: recyclingCode,
	}
}

func (c *CoroutineWorker) SetWork(task task.Task) {
	//原本状态为可接收时
	if c.taskStatus.CompareAndSwap(Acceptable, Ready) {
		//之前是空闲状态，可以接收任务
		c.task = task
		//原本状态为执行
		c.taskStatus.CompareAndSwap(Ready, Running)
	}
}

func (c *CoroutineWorker) RunWork() {
	//c.one.Do(func() { //控制只能启动一次，不允许重复启动,不过启动命令本来就是内部调用一次，也用不着这个了吧
	go func() {
		//初始化的时候，会将状态设置为可启动
		for c.workerStatus.Load() { //状态是活着的时候
			if c.taskStatus.CompareAndSwap(Running, Execute) {
				c.task.RunTask()
				//活干完了，重置回待接收状态
				c.taskStatus.Store(Acceptable)
				//给通信信道发消息，让信道方便回收
				c.signal <- c.recyclingCode
			}
		}
	}()
	//})
}
func (c *CoroutineWorker) OverWorker() {
	//让协程自己结束
	c.workerStatus.Store(false)
}
