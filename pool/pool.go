package pool

import (
	"github.com/Diana-Fox/CoroutinePool/task"
	"github.com/Diana-Fox/CoroutinePool/worker"
	"github.com/google/uuid"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// 池子定义
type Pool interface {
	//NewPool(coreSize int, maxSize int,
	//	keepAliveTime int, time time.Duration,
	//	taskQueue chan task.Task, reject func()) (Pool, error)
	Submit(task task.Task) //提交任务
	ShutDown()             //关闭
}

func NewPool(coreSize int, maxSize int,
	keepAliveTime int, time time.Duration,
	taskQueue chan task.Task, reject func()) (Pool, error) {
	return &GoroutinePool{}, nil
}

const (
	PoolInit        = 0 //原始状态
	PoolShutDown    = 1 //池子关闭
	PoolWaitDestroy = 2 //任务已经全部处理完毕，协程可以进行销毁回收了
	PoolDestroyed   = 3 //监测自毁的也可以自毁了
)

type GoroutinePool struct {
	//内部的一些控制
	runningWorkers map[string]worker.Worker //这个用来存放待回收的协程呢？用map是为了快速往空闲调度
	idleWorkers    []worker.Worker          //这个是空闲可使用的协程,用这个类型，方便回收的时候做加速
	currentSize    atomic.Uint32            //当前存活的协程数量
	signal         chan string              //池和worker通信的信号,方便池子回收
	poolStatus     atomic.Uint32
	mx             sync.Mutex
	//-----------对用户暴露的参数--------------
	coreSize      int
	maxSize       int
	keepAliveTime int
	time          time.Duration  //存活时间
	tasks         chan task.Task //应当限制大小
	reject        func()
}

func (g *GoroutinePool) Submit(task task.Task) {
	if g.poolStatus.Load() >= PoolShutDown {
		//关闭了，不能再往里面加了,剩下的一些状态变化是为了自毁使用
		//todo 这里是直接抛异常还是怎么样，待考虑
	} //这个方法，改成轻逻辑呢？把重逻辑放到调度方法中去

	select {
	case g.tasks <- task:
		return
	default:
		g.reject() //满了，去执行拒绝策略
	}
	//g.mx.Lock()
	//defer g.mx.Unlock() //执行完就释放掉
	////没够核心数
	//if g.currentSize.Load() < uint32(g.coreSize) {
	//	//二话不说，增加协程
	//	//todo 这里要控制线程安全，需要上锁，再在这里还要再次判断一下协程数量是否达到上限
	//	code := uuid.New().String()
	//	worker.NewWorker(g.signal, code)
	//	//然后为协程捆绑上当前任务，并且执行
	//	//当前协程需要在运行协程上登记
	//	//为总协程数量做++操作
	//
	//} else if g.currentSize.Load() < uint32(g.maxSize) {
	//	//todo 已经超过核心线程数了，但是没达到最大线程数，考虑开辟新的，还是放入队列，后面再讨论
	//} else {
	//	//g.tasks <- task
	//	select {
	//	case g.tasks <- task:
	//		return
	//	default:
	//		g.reject() //满了，去执行拒绝策略
	//	}
	//}
}

func (g *GoroutinePool) ShutDown() {
	//TODO implement me
	panic("implement me")
}

// 去执行任务队列里面的活
func (g *GoroutinePool) coordinate() {
	go func() {
		tk := time.NewTicker(g.time)
		//去读取任务队列的任务
		for g.poolStatus.Load() < PoolWaitDestroy {
			select {
			case <-tk.C:
				//todo 去释放掉多余的协程
				g.resetting()
			case t := <-g.tasks:
				tk.Reset(g.time) //重置
				//todo 去拿协调器,拿到后去执行
				w := g.getWorker()
				for w == nil { //拿不到空闲就一直去尝试拿取
					//先把协程执行权让出去，因为拿不到空闲是需要等待别的执行完的
					runtime.Gosched()
					w = g.getWorker()
				}
				//拿到了，去设置任务，协程会自动去执行，能拿到，肯定是空闲的
				w.SetWork(t)
			}
		}
	}()
}

// 去拿一个空闲的worker
func (g *GoroutinePool) getWorker() worker.Worker {
	//g.mx.Lock()
	//defer g.mx.Unlock()
	if len(g.idleWorkers) > 0 {
		//element := g.idleWorkers.Front()
		//w := element.Value.(worker.Worker)
		//去取一个可以用的work
		w := g.idleWorkers[len(g.idleWorkers)]             //去取最后一个协程
		g.idleWorkers = g.idleWorkers[:len(g.idleWorkers)] //空闲协程少一个
		code := w.GetRecyclingCode()
		g.runningWorkers[code] = w //运行协程多一个
		return w
	} else if g.currentSize.Load() < uint32(g.maxSize) {
		//没到最大线程数，去开新的协程
		w := worker.NewWorker(g.signal, uuid.New().String())
		g.runningWorkers[w.GetRecyclingCode()] = w
		g.currentSize.Add(1) //开启的总协程数要多一个
		return w
	} else {
		//没拿到
		return nil
	}
}

// 恢复到空闲中去
func (g *GoroutinePool) recovery() {
	go func() {
		//任务没处理完之前，都要将空闲线程归位，用于调度
		for g.poolStatus.Load() < PoolWaitDestroy {
			select {
			case s := <-g.signal:
				//对这两个集合的操作，必须上锁
				g.mx.Lock()
				//取出要归位的空闲协程
				w := g.runningWorkers[s]
				w.RunWork() //启动
				g.idleWorkers = append(g.idleWorkers, w)
				//g.idleWorkers.PushBack(w) //重新放回去
				g.mx.Unlock()
			}
		}
	}()
}

// 重置协程的个数,为什么统一回收呢，因为当有一个
func (g *GoroutinePool) resetting() {
	//todo 只有在长时间没有任务的时候才会重置，所以协程必然全是空闲的
	if g.currentSize.Load() > uint32(g.coreSize) {
		g.mx.Lock()
		releases := g.idleWorkers[g.coreSize:] //得到所有需要被释放掉的协程
		g.idleWorkers = g.idleWorkers[:g.coreSize]
		g.mx.Unlock()
		go func() {
			//todo 去挨个结束掉他们
			for i := 0; i < len(releases); i++ {
				releases[i].OverWorker()
			}
			//执行完成的时候，releases对象会被回收掉
		}()
	}
}
