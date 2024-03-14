package pool

import (
	"fmt"
	"github.com/Diana-Fox/CoroutinePool/task"
	"github.com/Diana-Fox/CoroutinePool/worker"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"time"
)

// 池子定义
type Pool interface {
	Submit(task task.Task) //提交任务
	ShutDown()             //关闭
}

func NewPool(coreSize int, maxSize int, keepLive time.Duration,
	taskQueue chan task.Task, reject func()) (Pool, error) {
	var poolStatus atomic.Uint32
	poolStatus.Store(PoolRunning) //池子现在是
	gp := GoroutinePool{
		workers:    make(map[string]worker.Worker),
		signal:     make(chan string, maxSize), //最多和协程数量一样多的信息过来，且概率极低
		poolStatus: poolStatus,
		coreSize:   coreSize,
		keepLive:   keepLive,
		maxSize:    maxSize,
		tasks:      taskQueue,
		reject:     reject,
	}
	gp.poolMonitor() //开启对池子的监控
	return &gp, nil
}

const (
	PoolRunning     = 0 //原始状态
	PoolShutDown    = 1 //池子关闭
	PoolWaitDestroy = 2 //任务已经全部处理完毕，协程可以进行销毁回收了
)

type GoroutinePool struct {
	//内部的一些控制
	workers map[string]worker.Worker //所有的协程
	//currentSize atomic.Uint32            //当前存活的协程数量
	signal     chan string //池和worker通信的信号,方便池子回收
	poolStatus atomic.Uint32
	mx         sync.Mutex
	//-----------对用户暴露的参数--------------
	coreSize int
	maxSize  int
	keepLive time.Duration  //存活时间
	tasks    chan task.Task //应当限制大小
	reject   func()
}

func (g *GoroutinePool) Submit(task task.Task) {
	if g.poolStatus.Load() > PoolRunning {
		//关闭了，不能再往里面加了
		//todo 这里是直接抛异常还是怎么样，待考虑，理论上这里应该有异常作为提醒
	} //
	//todo 去判断当前有几个worker滴干活
	if len(g.workers) < g.coreSize {
		//去增加worker去
		g.addWorker()
	} else if len(g.workers) < g.maxSize {
		//todo 暂时设定，达到核心线程数了，但是没到最大，暂定只有任务数是当前协程数的三倍再考虑开协程
		if len(g.tasks) > 3*len(g.workers) {
			g.addWorker()
		}
	}
	select {
	case g.tasks <- task:
		//成功提交任务了
		return
	default:
		g.reject() //去执行拒绝策略
	}
}

func (g *GoroutinePool) ShutDown() {
	//关闭池子，只能有一个线程成功，别的都白瞎
	g.poolStatus.CompareAndSwap(PoolRunning, PoolShutDown)
}

// 增加worker数量
func (g *GoroutinePool) addWorker() {
	g.mx.Lock()
	code := uuid.New().String()
	w := worker.NewWorker(g.tasks, g.signal, code, g.keepLive)
	w.RunWork()         //开始干活
	g.workers[code] = w //放进去
	g.mx.Unlock()
}

// 去监控池子状态，控制资源回收
func (g *GoroutinePool) poolMonitor() {
	go func() {
		//在自毁状态之前都需要监控
		for g.poolStatus.Load() != PoolWaitDestroy { //池子没到自毁之前
			//池子
			//运行中的概率最高，往前放放
			if g.poolStatus.Load() == PoolRunning {
				select {
				case code := <-g.signal:
					//要是当前协程数量比核心协程多，并且协程数量还是任务数量的2倍时，就自我了结
					//这个判断主要是为了保住核心协程，避免重复开启核心协程
					if g.coreSize < len(g.workers) && len(g.workers)*2 > len(g.tasks) {
						g.workOver(code)
					}
				}
			} else if g.poolStatus.Load() == PoolShutDown {
				if len(g.workers) == 0 && len(g.signal) == 0 {
					//关闭了，并且worker全自毁了,跳出这个循环，去关闭信道
					fmt.Println("协程全自毁了,该去关信道了")
					g.poolStatus.Store(PoolWaitDestroy)
					break
				}
				//协程池已经不接收新任务了，这时的超时，等于是可以直接回收了
				select {
				case code := <-g.signal:
					fmt.Printf("接收到的code:%s\n", code)
					g.workOver(code) //work去自毁
				}
			}
		}
		//fmt.Printf("信号的长度%d", len(g.signal))
		//fmt.Printf("协程的长度%d", len(g.tasks))
		close(g.tasks)
		close(g.signal)
	}()
}

// 协程狗带
func (g *GoroutinePool) workOver(code string) {
	g.mx.Lock()
	defer g.mx.Unlock()
	//去回收掉这个协程
	w := g.workers[code]
	//fmt.Println(w.GetRecyclingCode())
	w.OverWorker()
	//移除出去
	delete(g.workers, w.GetRecyclingCode())
}
