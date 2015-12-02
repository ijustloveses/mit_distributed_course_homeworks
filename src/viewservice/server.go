package viewservice

import "net"
import "net/rpc"
import "log"
import "time"
import "sync"
import "fmt"
import "os"
import "sync/atomic"

type ViewServer struct {
	mu       sync.Mutex
	l        net.Listener
	dead     int32 // for testing
	rpccount int32 // for testing
	me       string

	// Your declarations here.
	v     View           // view (Primary/Backup/Viewnum)
	nv    View           // new view, view would change until Primary confirmed
	nodes map[string]int // a dict of servers that once connected viewserver
}

//
// server Ping RPC handler.
//
func (vs *ViewServer) Ping(args *PingArgs, reply *PingReply) error {

	// Your code here.
	vs.mu.Lock()
	if vs.v.Viewnum == vs.nv.Viewnum {
		if vs.v.Primary == args.Me || vs.v.Backup == args.Me {
			if vs.v.Viewnum == args.Viewnum {
				vs.nodes[args.Me] = 0
			} else {
				vs.nodes[args.Me] = DeadPings
			}
		}

		if vs.nv.Primary == "" {
			vs.nv.Primary = args.Me
			vs.nv.Viewnum++
		} else if vs.nv.Backup == "" && vs.nv.Primary != args.Me {
			vs.nv.Backup = args.Me
			vs.nv.Viewnum++
		}
		reply.View = vs.v
	} else {
		if vs.nv.Primary == args.Me {
			vs.v = vs.nv
			reply.View = vs.nv
		} else {
			reply.View = vs.v
		}
	}

	vs.mu.Unlock()

	return nil
}

//
// server Get() RPC handler.
//
func (vs *ViewServer) Get(args *GetArgs, reply *GetReply) error {

	// Your code here.
	reply.View = vs.v

	return nil
}

//
// tick() is called once per PingInterval; it should notice
// if servers have died or recovered, and change the view
// accordingly.
//
func (vs *ViewServer) tick() {

	// Your code here.
	vs.mu.Lock()
	for node, cnt := range vs.nodes {
		vs.nodes[node] = cnt + 1
	}

	var viewchanged = 0

	if vs.nodes[vs.v.Primary] >= DeadPings && vs.nv.Viewnum == vs.v.Viewnum {
		vs.nv.Primary = ""
		if vs.v.Backup != "" && vs.nodes[vs.v.Backup] < DeadPings {
			vs.nv.Primary = vs.v.Backup
		} else {
			for node, cnt := range vs.nodes {
				if node != vs.v.Primary && node != vs.v.Backup && cnt < DeadPings {
					vs.nv.Primary = node
					break
				}
			}
		}
		vs.nv.Backup = ""
		viewchanged = 1
	}

	if vs.nv.Primary != "" && (vs.nv.Backup == "" || vs.nodes[vs.v.Backup] >= DeadPings) {
		for node, cnt := range vs.nodes {
			if node != vs.nv.Primary && cnt < DeadPings {
				vs.nv.Backup = node
				viewchanged = 1
				break
			}
		}
	}

	if viewchanged == 1 {
		vs.nv.Viewnum++
	}
	vs.mu.Unlock()
}

//
// tell the server to shut itself down.
// for testing.
// please don't change these two functions.
//
func (vs *ViewServer) Kill() {
	atomic.StoreInt32(&vs.dead, 1)
	vs.l.Close()
}

//
// has this server been asked to shut down?
//
func (vs *ViewServer) isdead() bool {
	return atomic.LoadInt32(&vs.dead) != 0
}

// please don't change this function.
func (vs *ViewServer) GetRPCCount() int32 {
	return atomic.LoadInt32(&vs.rpccount)
}

// for debug
func (vs *ViewServer) debug() {
	fmt.Printf("main primary: %s\n", vs.v.Primary)
	fmt.Printf("main backup: %s\n", vs.v.Backup)
	fmt.Printf("main viewnum: %d\n", vs.v.Viewnum)
	fmt.Printf("new primary: %s\n", vs.nv.Primary)
	fmt.Printf("new backup: %s\n", vs.nv.Backup)
	fmt.Printf("new viewnum: %d\n", vs.nv.Viewnum)
}

func StartServer(me string) *ViewServer {
	vs := new(ViewServer)
	vs.me = me
	// Your vs.* initializations here.
	vs.v.Primary = ""
	vs.v.Backup = ""
	vs.v.Viewnum = 0
	vs.nv = vs.v
	vs.nodes = make(map[string]int)

	// tell net/rpc about our RPC server and handlers.
	rpcs := rpc.NewServer()
	rpcs.Register(vs)

	// prepare to receive connections from clients.
	// change "unix" to "tcp" to use over a network.
	os.Remove(vs.me) // only needed for "unix"
	l, e := net.Listen("unix", vs.me)
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	vs.l = l

	// please don't change any of the following code,
	// or do anything to subvert it.

	// create a thread to accept RPC connections from clients.
	go func() {
		for vs.isdead() == false {
			conn, err := vs.l.Accept()
			if err == nil && vs.isdead() == false {
				atomic.AddInt32(&vs.rpccount, 1)
				go rpcs.ServeConn(conn)
			} else if err == nil {
				conn.Close()
			}
			if err != nil && vs.isdead() == false {
				fmt.Printf("ViewServer(%v) accept: %v\n", me, err.Error())
				vs.Kill()
			}
		}
	}()

	// create a thread to call tick() periodically.
	go func() {
		for vs.isdead() == false {
			vs.tick()
			time.Sleep(PingInterval)
		}
	}()

	return vs
}
