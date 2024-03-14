package CoroutinePool

import (
	"fmt"
	"github.com/Diana-Fox/CoroutinePool/pool"
	"github.com/Diana-Fox/CoroutinePool/task"
	"sync"
	"testing"
	"time"
)

func TestPool(t *testing.T) {
	tasks := make(chan task.Task, 5)
	var wg sync.WaitGroup
	wg.Add(20)
	p, err := pool.NewPool(1, 5, time.Second*2, tasks, func() {
		fmt.Println("任务被拒绝了")
	})
	if err != nil {
		fmt.Println("创建失败")
		return
	}
	for i := 0; i < 20; i++ {
		p.Submit(task.NewTask(func() {
			fmt.Printf("执行任务:%d\n", i)
			wg.Done()
		}))
		if i%5 == 0 {
			//放一次停1s呢，避免太快了直接被拒绝了
			time.Sleep(time.Second)
		}
	}
	wg.Wait()
	//time.Sleep(time.Second * 10)
	//p.Submit(task.NewTask(func() {
	//	fmt.Println("执行一下循环外的")
	//}))
	//todo 上面是正向流程，已经输出正确结果了，下面会是协程池关闭
	p.ShutDown()
	fmt.Println("已经设置为关闭了")
	time.Sleep(time.Second * 5)
	fmt.Println("查看一下")
	//fmt.Println("任务都执行完了")

}
