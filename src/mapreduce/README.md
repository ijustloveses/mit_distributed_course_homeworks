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
测试结果要求：输入文件和结果文件一致；并会观察 mr.stats也即 workers 的运行信息，要求每个 Worker 都至少被 rpc 请求过一次

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
    |                                                                                               |-- mr.DoenChannel <- true
    |
    |----------------------------------------------------------------------------------------------------------------------> two Worker goroutine，并向 Master 发送注册
    |
    |-- 等待 mr.DoneChannel，也即等待算法执行完毕
    |-- 判断结果是否正确

看到，当主进程接到 mr.DoneChannel 时，Mapreduce goroutine 肯定已经运行结束，因为最后一句执行语句 mr.DoenChannel <- true 已经执行过了
故此，CleanupRegistration() 肯定已经执行过了，那么 Master goroutine 肯定已经结束，因为会接收到 Shutdown 请求，mr.alive 被设置为 false
当然，更早的 mr.RunMaster 函数也执行过了，这个函数的最后，也就是完成 MapReduce 后会调用 KillWorker 来杀死 Workers，故此 Workers 也都死掉了
看到全部 goroutine 都会正常完结，不会有任何问题

那么，整个流程缺失的部分为：
Worker goroutine 启动向 Master 注册，Master 需要做一定的处理工作，记录 Worker 信息
最重要的 mr.RunMaster() 函数，这个函数是整个框架的核心逻辑
