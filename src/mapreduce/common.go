package mapreduce

import "fmt"
import "net/rpc"

const (
	Map    = "Map"
	Reduce = "Reduce"
)

type JobType string

// RPC arguments and replies.  Field names must start with capital letters,
// otherwise RPC will break.

// 可以参考 mapreduce.go 中 DoMap & DoReduce 的参数，来对应下面的参数
type DoJobArgs struct {
	File      string
	Operation JobType // Map or Reduce
	JobNumber int     // this job's number，这个不是总数，而是第几个
	// map 任务需要知道 reduce 的总数，以便生成对应的文件
	// reduce 任务需要知道 map 的总数，以便合并多个map生成的文件
	NumOtherPhase int // total number of jobs in other phase (map or reduce)
}

type DoJobReply struct {
	OK bool
}

type ShutdownArgs struct {
}

type ShutdownReply struct {
	Njobs int
	OK    bool
}

type RegisterArgs struct {
	Worker string
}

type RegisterReply struct {
	OK bool
}

//
// call() sends an RPC to the rpcname handler on server srv
// with arguments args, waits for the reply, and leaves the
// reply in reply. the reply argument should be the address
// of a reply structure.
//
// call() returns true if the server responded, and false
// if call() was not able to contact the server. in particular,
// reply's contents are valid if and only if call() returned true.
//
// you should assume that call() will time out and return an
// error after a while if it doesn't get a reply from the server.
//
// please use call() to send all RPCs, in master.go, mapreduce.go,
// and worker.go.  please don't change this function.
//
func call(srv string, rpcname string,
	args interface{}, reply interface{}) bool {
	c, errx := rpc.Dial("unix", srv)
	if errx != nil {
		return false
	}
	defer c.Close()

	err := c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
