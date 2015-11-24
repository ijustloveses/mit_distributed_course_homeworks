package mapreduce

import "container/list"
import "fmt"

type WorkerInfo struct {
	address string
	// You can add definitions here.
}

type JobInfo struct {
	jobid int
	res   bool
}

// Clean up all workers by sending a Shutdown RPC to each one of them Collect
// the number of jobs each work has performed.
func (mr *MapReduce) KillWorkers() *list.List {
	l := list.New()
	for _, w := range mr.Workers {
		DPrintf("DoWork: shutdown %s\n", w.address)
		args := &ShutdownArgs{}
		var reply ShutdownReply
		ok := call(w.address, "Worker.Shutdown", args, &reply)
		if ok == false {
			fmt.Printf("DoWork: RPC %s shutdown error\n", w.address)
		} else {
			// 把该 worker 执行过的 jobs 数在 reply 返回，会被最终set到 mr.stats
			l.PushBack(reply.Njobs)
		}
	}
	return l
}

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
	jobchan := make(chan JobInfo)
	domap := func(jobid int) {
		// 取出 worker
		worker := <-mr.registerChannel
		// map 需要 nReduce 把结果拆分
		ret := dojob(worker, Map, mr.nReduce, jobid)
		// job结果
		jobchan <- JobInfo{jobid, ret}
		// 再次进 Channel, 这里在耗尽任务时会阻塞
		mr.registerChannel <- worker
	}
	dored := func(jobid int) {
		worker := <-mr.registerChannel
		// reduce 需要 nMap 来收集全部 map 过来的数据
		ret := dojob(worker, Reduce, mr.nMap, jobid)
		jobchan <- JobInfo{jobid, ret}
		mr.registerChannel <- worker
	}

	// map phase
	for i := 0; i < mr.nMap; i++ {
		go domap(i)
	}
	var succ = 0
	for succ < mr.nMap {
		ret := <-jobchan
		if ret.res {
			succ++
		} else {
			go domap(ret.jobid)
		}
	}

	// reduce phase
	for i := 0; i < mr.nReduce; i++ {
		go dored(i)
	}
	succ = 0
	for succ < mr.nReduce {
		ret := <-jobchan
		if ret.res {
			succ++
		} else {
			go dored(ret.jobid)
		}
	}
	// Your code here
	return mr.KillWorkers()
}
