package worker

import (
	"fmt"
	"github.com/Diana-Fox/CoroutinePool/task"
	"sync/atomic"
	"time"
)

type Worker interface {
	RunWork()                 //启动协程
	GetRecyclingCode() string //获取控制码
	OverWorker()              //结束协程
}

const (
	Acceptable = iota + 1 //可以接收任务
	Ready                 //就绪态
	Running               //可以被执行
	Execute               //执行态
)

type CoroutineWorker struct {
	tasks         chan task.Task //要有个任务
	workerStatus  atomic.Bool    //任务协程的生死
	signal        chan string    //方便任务完成的时候，归入空闲协程
	recyclingCode string         //回收码,为了实现指定协程的回收
	time          time.Duration  //提醒时间
}

func NewWorker(tasks chan task.Task, signal chan string, recyclingCode string, time time.Duration) Worker {
	var workerStatus atomic.Bool
	workerStatus.Store(true) //初始化
	return &CoroutineWorker{
		tasks:         tasks,
		workerStatus:  workerStatus,
		signal:        signal,
		recyclingCode: recyclingCode,
		time:          time,
	}
}
func (c *CoroutineWorker) RunWork() {
	go func() {
		tk := time.NewTicker(c.time)
		for c.workerStatus.Load() { //协程还没被弄死
			select {
			case <-tk.C:
				//去通知池子这个协程可以回收了，但是自己不回收,因为可能并不想回收这个协程
				c.signal <- c.recyclingCode
				tk.Reset(c.time)
			case t := <-c.tasks:
				//if t != nil {
				t.RunTask()
				fmt.Printf("协程执行任务%s\n", c.recyclingCode)
				tk.Reset(c.time)
				//} //可能接收到nil
			default:
				continue
			}
		}
		fmt.Printf("协程销毁了%s\n", c.recyclingCode)
	}()
}

func (c *CoroutineWorker) GetRecyclingCode() string {
	return c.recyclingCode
}

func (c *CoroutineWorker) OverWorker() {
	//让协程自己结束
	//if c.workerStatus.CompareAndSwap(true, false) {
	//	fmt.Printf("已接收结束信号%s", c.recyclingCode)
	//}
	c.workerStatus.Store(false)
}
