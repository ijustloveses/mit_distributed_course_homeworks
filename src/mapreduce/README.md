Course Schedule
===================
http://nil.csail.mit.edu/6.824/2015/schedule.html 

MapReduce
===========
http://nil.csail.mit.edu/6.824/2015/notes/l01.txt
http://nil.csail.mit.edu/6.824/2015/labs/lab-1.html
http://nil.csail.mit.edu/6.824/2015/papers/mapreduce.pdf

###Part.1 单机单进程模式
mapreduce/mapreduce.go RunSingle()
**输入**
map数 nMap, reduce数 nReduce, 数据文件 file, Map 函数和 Reduce 函数

**流程**
由于是单机单进程模式，故此不会启动 listener，一个进程顺序执行下面的流程：
1. Split - 目的是为每个 map 生成一个处理文件，记为 file-{map_index}；
           使用 bufio.NewScanner 逐行扫描，一旦超过 (Size/nMap) * (map_index + 1) 即生成一个 map 处理文件
2. DoMap - 每个 map 节点处理自己的对应文件，为每个 reduce 生成一个处理文件，记为 file-{map_index}-{red_index}
           具体方法是把整个文件读入字符串，送入 Map 函数处理，生成 list of KeyValue
           对每个 reduce 节点 r，都遍历一遍 list，把其中 hash(key) == r 的 KeyValue 通过 json.NewEncoder 写入 reduce 文件
3. DoReduce - DoMap 中每个 map 节点生成 nReduce 个处理文件，而本函数对应一个 reduce 节点，共计需要处理 nMap 个该节点对应的文件
              具体方法是维护一个 kvs，然后遍历 nMap 个文件，使用 json.NewDecoder 读取得到每个文件中的所有 KeyValue，合并到 kvs 中
              然后对 kvs 按 key 排序，使用 Reduce 函数处理该 key 对应的 Values 列表
              最后，通过 json.NewEncoder 把处理之后的 key 和新的 value 写入该 reduce 节点的结果文件 file-res-{red_index} 中
4. Merge - 类似 DoReduce，维护一个 kvs，然后使用 json.NewDecoder 读取全部 nReduce 个节点生成的 file-res 文件
           把每个文件中的所有 KeyValue 合并到 kvs 中，最后按 key 排序，并使用 bufio.NewWriter 写入最终结果文件 mrtmp-file 中
           由于每个 reduce 节点处理的都是 hash(key) == r 的 KeyValue，故此 reduce 节点之间不会有 key 的冲突，故此 Merge 的合并就是简单的累积而已
上面的算法，是一个框架，和具体的 Map 和 Reduce 函数无关，这两个是看具体的业务来实现
中间文件共有：
    map节点处理文件 (nMap 个，Split 的结果)
    reduce节点处理文件 (nMap * nReduce 个，DoMap的结果)
    Merge 的处理文件 (nReduce 个，DoReduce的结果)
最终结果文件只有一个，就是 Merge 的结果

RunSingle looks like:
func RunSingle(nMap int, nReduce int, file string,
        Map func(string) *list.List,
        Reduce func(string, *list.List) string) {
    mr := InitMapReduce(nMap, nReduce, file, "")
    mr.Split(mr.file)
    for i := 0; i < nMap; i++ {
        DoMap(i, mr.file, mr.nReduce, Map)
    }
    for i := 0; i < mr.nReduce; i++ {
        DoReduce(i, mr.file, mr.nMap, Reduce)
    }
    mr.Merge()
}

**lab**
该模式下 lab 作业的要求是实现 WordCount 的 Map & Reduce 函数逻辑
为了更简单的测试这两个函数，我基于 mapreduce.go 实现了 test_mapreduce_func.go 文件，去掉了拆分和合并(相当于一个map + 一个reduce)

为了运行测试脚本，首先在 ~/.bashrc 中加入当前目录
export GOPATH=/vagrant/gopath:/home/cuiyong/workspace/myproj/mit_distributed_course_homeworks

运行如下：
$ cd src/main
$ go run wc.go masterequential
结果文件为 mrtmp.kjv12.txt ，通过 sort -n -k2 |tail -10 可以来查看 top 10
unto: 8940
he: 9666
shall: 9760
in: 12334
that: 12577
And: 12846
to: 13384
of: 34434
and: 38850
the: 62075

或者使用测试脚本
$ sh ./test-wc.sh
确保最后输出 Passed test

使用下面语句来清理临时和结果文件
$ rm mrtmp.*


###Part.2 Goroutine + unix domain socket 模拟多机模式
保持 Part.1 中的基本单元 Split() DoMap() DoReduce 和 Merge() 不变，把整个流程使用 goroutine + rpc 串起来
其实是由于仍然是单机，上述的函数才可能保持不变，因为通过 filename + map 号 + reduce 号 可以定位全部的中间文件

为了研究我们需要做什么，我们先研究一下代码

**common.go**
定义了操作类型，包括 Map & Reduce 两种，其实他们定义了 worker 所要进行任务的类型
定义了3 类 rpc 任务，包括 DoJob，Shutdown 和 Register，每种任务对应一个 Args 结构和一个 Reply 结构
其中，我们可以参考 mapreduce.go 中 DoMap & DoReduce 的参数，来对应 DoJobArgs 结构中的成员
最后，封装了 rpc 调用函数 call(srv, rpcname, args, reply)
srv 或者是 master 或者是 worker， rpcname 即函数名，args 是函数参数，对应 XXArgs 结构, reply 为**结构指针**，负责返回结果

**mapreduce.go**
Part.1 中我们已经研究过这个文件对应的单线程部分了，而且 MapReduce 算法框架基本单元并没有改变，故此，这里只研究 goroutine + rpc 部分
首先 MapReduce 结构体中，包含了 MasterAddress，定义了 Master 节点的 unix domain socket; 还包含 registerChannel 用于 worker 节点的注册；
DoneChannel 用于完成整个流程时的通知；以及 stats 这个是一个 list.List 指针收集 workers 的运行信息 和 Workers 词典收集了所有的 Worker

提供了 Register 和 Shutdown 两个 rpc 接口供外部调用，显然，前者由 worker 来调用注册，后者由主程序调用结束运行

提供了 Master 节点的启动函数，为 Master 注册了一个 rpc server，监听后脱离主程序，使用 goroutine 接收连接，收到连接后再使用 goroutine 处理连接
至于到底如何处理，实际上由上面说过的 call 函数封装住了，其实就是外部 Dial 之后，Call(rpcname, args, reply); 然后 goroutine 收到 Call 后调用对应的 rpcname 函数

提供 CleanupRegistration() 函数，由主程序调用来向 Master 发送 Shutdown 请求

最后提供 Goroutine + unix domain socket 下的 MapReduce 算法框架 in Run()：
    mr.Split(mr.file)
    mr.stats = mr.RunMaster()
    mr.Merge()
    mr.CleanupRegistration()
    mr.DoneChannel <- true

由此我们看到整个流程是启动了 mapreduce 框架实例 (mr) 之后，框架主程序来 Split 文件，然后让 Master 接手进行 MapReduce 计算
然后主程序来做最后的 Merge，通知 Master 节点退出，最后向 mr.DoneChannel 发送 done 来结束整个程序

其中，显然大部分都已经实现，未知的部分就是 mr.RunMaster() 函数

**test_test.go**
那么，上面的 mapreduce 框架实例 mr 是如何创建的呢？向 mr.DoneChannel 发送 done 是如何结束整个程序的呢？
这两部分和算法框架无关，依不同程序而异，故此在测试脚本 test_test.go 文件中

测试内容：生成一个包含 0~99999 共计 10万行的文件作为初始文件；提供了 MapFunc 用于收集文件中的每个字符并 Emit {字符, ""} KeyValue 值；
提供了 ReduceFunc，简单的打印结果，Reduce 的结果置为 ""，故此对于输入文件，经过 Map & Reduce 后得到的仍然还是和输入文件一样的结果文件
测试结果要求：输入文件和结果文件一致；并会观察 mr.stats也即 workers 的运行信息，要求每个 Worker 都至少执行过一次任务

Part.2 的测试流程为 TestBasic 函数，定义如下：
    mr := setup()               <--- 这里创建并启动 mapreduce 实例 mr，其中启动了 Master 节点，并启动 goroutine 运行框架的 Run()
    for i := 0; i < 2; i++ {    <--- 这里启动了两个 worker
        go RunWorker(mr.MasterAddress, port("worker"+strconv.Itoa(i)), MapFunc, ReduceFunc, -1)
    }
    <-mr.DoneChannel            <--- 看到，启动 Workers 之后就一直等待这 mr.DoneChannel 的消息，一旦收到 True 运行结束
    check(t, mr.file)           <--- 开始检查是否运行正确
    checkWorker(t, mr.stats)

**worker.go**
上面看到，一共就启动了两个 workers 来做全部的 mapreduce 工作，那么看看 worker 是如何定义的
Worker 结构体对应每个 Worker，维护 name, MapFunc, ReduceFunc, listener；还有两个整数，一个是 nRPC，一个是 nJobs

提供 DoJob rpc 接口，根据传入的 DoJobArgs 来决定是调用 DoMap 还是 DoReduce 函数
提供 Shutdown rpc 接口，接收 Master 节点的消息来进行 Shutdown，看到 Shutdown 之后 wk.nRPC = 1

提供函数 Register 去调用 Master节点的 Register rpc 接口实现注册

Worker 节点的 loop 函数 RunWorker() 为：
    初始化 nRPC = -1
    初始化 MapFunc, ReduceFunc, name，然后向 Master 节点注册
    如果 nRPC !=0 循环等待任务，每等到一个就让 nRPC 递减 1；之前 Shutdown 时会让 nRPC = 1，也就是说一旦再减一就是 0 了，于是会跳出循环
    同时让 nJobs +=1 就是表示它又接收一项任务


**lab master.go**
master.go 中已经实现了 KillWorkers 函数，是 RunMaster 函数调用的最后也就是完成了 MapReduce 之后来调用，向 Workers 发送 Shutdown 请求
Worker 需要把其 nJobs 成员放到 reply 中返回，作为 RunMaster 最终的返回设置给 mr.stats，最终这个 nJobs 会被 check 来判断是否 worker 曾执行过任务

经过上面的分析，我们可以开始实现 lab 作业了，显然切入点是 test_test.go 文件; 整理一下目前我们已经有的框架和调用，如下：

主程序进程 test_test.go::TestBasic()
    |-- 调用 mr := setup()
    |     |-- 调用 MakeMapReduce()
    |           |-- 调用 mr.StartRegistrationServer()
    |           |     |-- 创建 Rpc，并 Listen
    |           |     |-------------------------------------> Master goroutine 等待请求
    |           |
    |           |-- 调用 go mr.Run() ----------------------------------------------------------> MapReduce goroutine
    |                                                                                               |-- Split()
    |                                                                                               |-- mr.stats = mr.RunMaster()
    |                                                                                               |-- Merge()
    |                                                                                               |-- CleanupRegistration() 向 Master 发送 Shutdown
    |                                                                                               |-- mr.DoneChannel <- true
    |
    |----------------------------------------------------------------------------------------------------------------------> two Worker goroutine，并向 Master 发送注册
    |
    |-- 等待 mr.DoneChannel，也即等待算法执行完毕
    |-- 判断结果是否正确

看到，当主进程接到 mr.DoneChannel 时，Mapreduce goroutine 肯定已经运行结束，因为最后一句执行语句 mr.DoneChannel <- true 已经执行过了
故此，CleanupRegistration() 肯定已经执行过了，那么 Master goroutine 肯定已经结束，因为会接收到 Shutdown 请求，mr.alive 被设置为 false
当然，更早的 mr.RunMaster 函数也执行过了，这个函数的最后，也就是完成 MapReduce 后会调用 KillWorker 来杀死 Workers，故此 Workers 也都死掉了
看到全部 goroutine 都会正常完结，不会有任何问题

那么，整个流程缺失的部分为：
Worker goroutine 启动向 Master 注册，Master 需要做一定的处理工作，记录 Worker 信息
最重要的 mr.RunMaster() 函数，这个函数是整个框架的核心逻辑

第一个部分很简单，两步完成：
mapreduce.go::InitMapReduce() 中设置初始化 mr.Workers = make(map[string]*WorkerInfo)；注意必须先初始化才能正常访问赋值
mapreduce.go::Register() 中每次注册时设置 mr.Workers[args.Worker] = &WorkerInfo{args.Worker}

第二个部分也就是 RunMaster 的实现略有难度，第一个版本使用 sync.WaitGroup 来控制先做 Map 再做 Reduce 再做 KillWorkers，保证流程同步，如下：
    func (mr *MapReduce) RunMaster() *list.List {
        // 定义函数，给 worker 发送 DoJob 请求
        dojob := func(worker string, op JobType, othertotal int, jobid int) bool {
            var args DoJobArgs
            var reply DoJobReply
            args.File = mr.file
            args.NumOtherPhase = othertotal
            args.JobNumber = jobid
            args.Operation = op
            return call(worker, "Worker.DoJob", args, &reply)
        }
        // map phase
        var wg sync.WaitGroup
        wg.Add(mr.nMap)
        for i := 0; i < mr.nMap; i++ {
            go func(jobid int) {
                defer wg.Done()
                // 取出 worker
                worker := <-mr.registerChannel
                // map 需要 nReduce 把结果拆分
               dojob(worker, Map, mr.nReduce, jobid)
               // 再次进 Channel
               mr.registerChannel <- worker
           }(i)
       }
       wg.Wait()
       fmt.Println("----------- Map Phase Done !! ----------\n")
       // reduce phase
       wg.Add(mr.nReduce)     for i := 0; i < mr.nReduce; i++ {
           go func(jobid int) {
               defer wg.Done()
               worker := <-mr.registerChannel
               // reduce 需要 nMap 来收集全部 map 过来的数据
               dojob(worker, Reduce, mr.nMap, jobid)
               mr.registerChannel <- worker
           }(i)
       }
       wg.Wait()
       // Your code here
       return mr.KillWorkers() 
    }

但是，发现 Map 调用部分结束后 wg.Wait() 并不会正常返回，而是持续在等待，故此无法接下去继续 Reduce 流程，暂时不清楚为什么会这样

故此，放弃 WaitGroup 的方法，而采用 Channel；既然 mr.nMap 和 mr.nReduce 是已知的，那么就新建 Channel 等待固定数量个结果，如下：
    type JobInfo struct {
        jobid int
        res   bool
    }
    ........
    jobchan := make(chan JobInfo)
    // map phase
    for i := 0; i < mr.nMap; i++ {
        go func(jobid int) {
            // 取出 worker
            worker := <-mr.registerChannel
            // map 需要 nReduce 把结果拆分
            ret := dojob(worker, Map, mr.nReduce, jobid)
            // 再次进 Channel
            mr.registerChannel <- worker
            // job结果
            jobchan <- JobInfo{jobid, ret}
        }(i)
    }
    for i := 0; i < mr.nMap; i++ {
        <-jobchan
    }
    // reduce phase
    for i := 0; i < mr.nReduce; i++ {
        go func(jobid int) {
            worker := <-mr.registerChannel
            // reduce 需要 nMap 来收集全部 map 过来的数据
            ret := dojob(worker, Reduce, mr.nMap, jobid)
            mr.registerChannel <- worker
            jobchan <- JobInfo{jobid, ret}
        }(i)
    }
    for i := 0; i < mr.nReduce; i++ {
        <-jobchan
    }

然而，结果一样，仍然是 Map 调用完成，然后就等待在那里不继续 Reduce 过程了
**通过打印信息调试，发现是因为最后两个   goroutine 在 mr.registerChannel <- worker 的时候阻塞住了，为什么呢？**
看到，mr.registerChannel 是一个不带缓冲的普通 channel，做以下实验

func main() {
    regchan := make(chan int)
    regchan <- 1
    regchan <- 2
    fmt.Println("Done")
}
fatal error: all goroutines are asleep - deadlock!    看到死锁，插入 1 后，就无法再插入 2 了

那么，如果使用 goroutine 插入呢？
func main() {
    regchan := make(chan int)
    go func() {
        regchan <- 1
        fmt.Println("insert 1")
    }()
    go func() {
        regchan <- 2
        fmt.Println("insert 2")
    }()
    fmt.Println("Done")
}
Done    看到，由于没有等待 goroutine，故此 goroutine 的输出没有显示，而主线程返回打印 Done，那么 goroutine 发生了什么？我们等待 goroutine 看看

func main() {
    regchan := make(chan int)
    go func() {
        regchan <- 1
        fmt.Println("insert 1")
    }()
    go func() {
        regchan <- 2
        fmt.Println("insert 2")
    }()
    <-regchan
    <-regchan
    fmt.Println("Done")
}
insert 1
Done 
看到，没有输出 insert 2，为什么？是因为主程序运行到第二个 <-regchan 企图阻塞等待 regchan 的时候，其实第二个 goroutine 已经运行完毕了 ... 增加延时看看

func main() {
    regchan := make(chan int)
    go func() {
        regchan <- 1
        fmt.Println("insert 1")
    }()
    go func() {
        time.Sleep(5)
        regchan <- 2
        fmt.Println("insert 2")
    }()
    fmt.Println(<-regchan)
    fmt.Println(<-regchan)
    fmt.Println("Done")
}
insert 1
1
insert 2
2
Done   OK 了！ 说明可以用 goroutine 来向普通的 channel 插入多条数据，插入 channel 第一个后，在插入的值还没有被取出之前，其他的 goroutine 只能等待

那么，我们就明白了，以 Map 流程为例，因为有两个 worker 要向 channel 注册，那么必然有一个一直处于等待中；
当 Map 流程开启，会取出已注册 worker 来执行任务 -> 那么第二个 worker 得以注册成功，然后又被分配任务 -> 于是完成任务的 worker 注册回到 channel 中
然而，到了最后，当某一个 worker 回到 channel，而此时已经没有任务可以分配了，于是另一个 worker 完成任务后只能等待 channel，无法正常返回，导致了错误

重现一下：
func main() {
    regchan := make(chan int)       <--- 用来模拟阻塞插入的 channel
    syncchan := make(chan int)      <--- 用来同步以显示输出的 channel
    for i := 0; i < 5; i++ {
        go func(id int) {
            s := <-regchan          <--- 从 regchan 取出
            fmt.Println("No. ", s)
            regchan <- s            <--- 处理完再插入
            fmt.Println("Id. ", id)
            syncchan <- id          <--- 更新同步 channel
        }(i)
    }
    worker1 := func() {             <--- 实现两个 goroutine，来为 regchan 插入，其实是一个初始化以便让循环启动的作用
        regchan <- 1
        fmt.Println("insert 1")
    }
    worker2 := func() {
        regchan <- 2
        fmt.Println("insert 2")
    }
    go worker1()
    go worker2()
    for i := 0; i < 5; i++ {
        <-syncchan
    }
}
insert 1
insert 2
No.  1
Id.  0
No.  2
Id.  1
No.  1
Id.  2
No.  2
No.  1
fatal error: all goroutines are asleep - deadlock!    看到，运行到最后就发生死锁

注意，golang 中非缓冲channel 写逻辑是：
writing operations on a synchronous channel can only succeed in case there is a receiver ready to get this data
(这也是使用同步 channel (如上面的 syncchan) 来实现同步的原因，只有主流程最后从同步channel中读取数据时， goroutine 中插入同步 channel 的语句才能开始执行，故此不可能事先运行完毕)
故此，更准确的说，其实是当所有任务完成后，没有人再从注册 channel 中读取 worker 时，就无法再插入任务了;
也就是说，注册 channel 中平时都是空着的，除非有任务在等待 worker; 不要认为是注册 channel 平时常驻一个且仅一个 worker

如何修复？显然，使用带缓冲的 channel 是最简单的，如下：
func main() {
    regchan := make(chan int, 2)
    syncchan := make(chan int)
    for i := 0; i < 5; i++ {
        go func(id int) {
            s := <-regchan
            fmt.Println("No. ", s)
            regchan <- s
            fmt.Println("Id. ", id)
            syncchan <- id
        }(i)
    }
    worker1 := func() {
        regchan <- 1
        fmt.Println("insert 1")
    }
    worker2 := func() {
        regchan <- 2
        fmt.Println("insert 2")
    }
    go worker1()
    go worker2()
    for i := 0; i < 5; i++ {
        <-syncchan
    }
}
insert 1
insert 2
No.  1
Id.  0
No.  2
Id.  1
No.  1
Id.  2
No.  2
Id.  3
No.  1
Id.  4
完美输出!   然而， 对于 MapReduce 却不能用，**因为使用待缓冲的 channel，需要事先知道或者说写死共计有多少个   workers (当然也可以给一个很大的值)**
而 MapReduce 是可以动态注册 worker 数的，或者说多少个 workers 是用户决定，而不是 MapReduce 中定死的

那么，使用非缓冲的 channel 如何同步呢？ 很简单，把同步 channel 的插入放到 regchan 的插入之前即可
func main() {
    regchan := make(chan int)
    syncchan := make(chan int)
    for i := 0; i < 5; i++ {
        go func(id int) {
            s := <-regchan
            fmt.Println("No. ", s)
            fmt.Println("Id. ", id)
            syncchan <- id
            regchan <- s
        }(i)
    }
    worker1 := func() {
        regchan <- 1
        fmt.Println("insert 1")
    }
    worker2 := func() {
        regchan <- 2
        fmt.Println("insert 2")
    }
    go worker1()
    go worker2()
    for i := 0; i < 5; i++ {
        <-syncchan
    }
}
insert 1
insert 2
No.  1
Id.  0
No.  2
Id.  1
No.  1
Id.  2
No.  1
Id.  3
No.  2
Id.  4  完美！

回顾 regchan 阻塞过程，是因为本就有两个 worker 等待入 channel，然后启动任务，channel 出入均衡，直到任务耗尽，最后又有两个任务等待入channel
由于都是在 goroutine 中，故此，如果不同步，那也没关系，就是两个 goroutine 一直在等待，主程序会自行退出，也没什么了不起的
而且根据 master.go::RunMaster() 最后会主动向两个 workers 发送 shutdown 请求的，也不会造成什么问题

之所以会一致等待，或者报错 DeadLock，就是因为我们引入了同步 channel (syncchan 或者是 MapReduce 中的 jobchan)
这个 channel 和 regchan 不同，没有事先等待插入的任务队列，只有当 worker 完成了任务，**并把 worker 重新插入回 channel** 之后才会开始向同步 channel 插入
而在主流程中，从同步 channel 取出任务数相同数目的元素

看到，如果 syncchan <- id 的调用发生在 regchan <- s 之后，显然最后任务耗尽时有两个 goroutine 一直在后台默默等待 regchan <- s 
于是 syncchan <- id 就一直死等在后面，于是主流程不可能从 syncchan 中取出与任务数相同的元素，于是死锁！

但是一旦 syncchan <- id 插入调用发生在 regchan <- s 之前呢？显然主流程是可以从 syncchan 中读取到全部任务数量个元素的，于是主流程不会阻塞
最后就只有两个 goroutine 会一直在后台等待，直到主程序退出

综上，唯一的修改就是，在 Map phase 和 Reduce phase 中，把 jobchan <- JobInfo{jobid, ret}  提到  mr.registerChannel <- worker  之前

检验结果：
$ cd src/mapreduce
$ go test
............
  ... Basic Passed

看到 Basic Passed 就 OK 了，再后面是 Part.3 的部分了 ^-^



###Part.3 Fail tolerance with re-try
原则：
if the master's RPC to the worker fails, the master should re-assign the job given to the failed worker to another worker.
because jobs are idempotent, it doesn't matter if the same job is computed twice---both times it will generate the same output. 
don't have to handle failures of the master; we will assume it won't fail. 
master fault-tolerant is more difficult because it keeps persistent state that would have to be recovered in order to resume operations after a master failure. 
the later labs are devoted to this master fault-tolerant challenge.

先看第一个测试用例 TestOneFailure() 和 TestBasic 基本一致，除了第一个 worker 的初始化使用
go RunWorker(mr.MasterAddress, port("worker"+strconv.Itoa(0)), MapFunc, ReduceFunc, 10)  也就是说，初始化 nRPC 为 10
我们知道， worker 的 loop 是检查 nRPC != 0，而且每次接收一个 RPC 调用就将 nRPC 减 1，于是这个 worker0 一共就只能接收到 10 个请求就退出了 loop 循环，不再接收请求
于是，10 次之后，该 worker 不再响应 rpc 请求，这就类似 worker 失败，失去连接
我们要做的是重新分配失败的任务，当然，只有当任务被分配给仍然活跃的 worker 时才会正常被处理

再看第二个测试用例 TestManyFailures()，它每秒启动两个 workers，每个 worker 都只接收 10 个请求

任务是找到失败的任务，并重新分配；注意，并不是维护 workers
很简单，我们前面已经有了用于同步的 jobchan，其中传递的是 JobInfo{$jobid, $ret}，于是把失败的 jobid 重新跑就 ok 了

原来的同步逻辑是跑完了多少个 Map 或 Reduce 任务
    for i := 0; i < mr.nMap; i++ {
        <-jobchan
    }
那么，现在应该改为成功跑完多少个 Map 或 Reduce 任务，需要给成功的任务计数，并重跑失败的任务
    var succ = 0
    for succ < mr.nMap {
        ret := <-jobchan
        if ret.res {
            succ++
        } else {
            go domap(ret.jobid)
        }
    }

检验： 
$ go test
  ... Basic Passed
  ... One Failure Passed
  ... Many Failures Passed
PASS
ok      mapreduce       41.772s
