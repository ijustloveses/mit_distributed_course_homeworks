package viewservice

import "testing"
import "runtime"
import "time"
import "fmt"
import "os"
import "strconv"

func check(t *testing.T, ck *Clerk, p string, b string, n uint) {
	view, _ := ck.Get()
	if view.Primary != p {
		t.Fatalf("wanted primary %v, got %v", p, view.Primary)
	}
	if view.Backup != b {
		t.Fatalf("wanted backup %v, got %v", b, view.Backup)
	}
	if n != 0 && n != view.Viewnum {
		t.Fatalf("wanted viewnum %v, got %v", n, view.Viewnum)
	}
	if ck.Primary() != p {
		t.Fatalf("wanted primary %v, got %v", p, ck.Primary())
	}
}

func port(suffix string) string {
	s := "/var/tmp/824-"
	s += strconv.Itoa(os.Getuid()) + "/"
	os.Mkdir(s, 0777)
	s += "viewserver-"
	s += strconv.Itoa(os.Getpid()) + "-"
	s += suffix
	return s
}

func Test1(t *testing.T) {
	runtime.GOMAXPROCS(4)

	vshost := port("v")
	vs := StartServer(vshost)

	ck1 := MakeClerk(port("1"), vshost)
	ck2 := MakeClerk(port("2"), vshost)
	ck3 := MakeClerk(port("3"), vshost)

	//

	if ck1.Primary() != "" {
		t.Fatalf("there was a primary too soon")
	}

	// very first primary
	// 看到并不要求 Ping 之后马上更新 View
	fmt.Printf("Test: First primary ...\n")

	for i := 0; i < DeadPings*2; i++ {
		view, _ := ck1.Ping(0)
		if view.Primary == ck1.me {
			break
		}
		time.Sleep(PingInterval)
	}
	check(t, ck1, ck1.me, "", 1)
	fmt.Printf("  ... Passed\n")

	// very first backup
	fmt.Printf("Test: First backup ...\n")

	{
		vx, _ := ck1.Get()
		for i := 0; i < DeadPings*2; i++ {
			// ck1 仍然保持连接，按设计，View 的更改必须通知 Primary
			ck1.Ping(1)
			view, _ := ck2.Ping(0)
			if view.Backup == ck2.me {
				break
			}
			time.Sleep(PingInterval)
		}
		check(t, ck1, ck1.me, ck2.me, vx.Viewnum+1)
	}
	fmt.Printf("  ... Passed\n")

	// primary dies, backup should take over
	fmt.Printf("Test: Backup takes over if primary fails ...\n")

	{
		ck1.Ping(2)
		vx, _ := ck2.Ping(2)
		for i := 0; i < DeadPings*2; i++ {
			v, _ := ck2.Ping(vx.Viewnum)
			if v.Primary == ck2.me && v.Backup == "" {
				break
			}
			time.Sleep(PingInterval)
		}
		check(t, ck2, ck2.me, "", vx.Viewnum+1)
	}
	fmt.Printf("  ... Passed\n")

	// revive ck1, should become backup
	// 此时，其实相当于 case 2.
	fmt.Printf("Test: Restarted server becomes backup ...\n")

	{
		vx, _ := ck2.Get()
		ck2.Ping(vx.Viewnum)
		for i := 0; i < DeadPings*2; i++ {
			ck1.Ping(0)
			v, _ := ck2.Ping(vx.Viewnum)
			if v.Primary == ck2.me && v.Backup == ck1.me {
				break
			}
			time.Sleep(PingInterval)
		}
		check(t, ck2, ck2.me, ck1.me, vx.Viewnum+1)
	}
	fmt.Printf("  ... Passed\n")

	// start ck3, kill the primary (ck2), the previous backup (ck1)
	// should become the server, and ck3 the backup.
	// this should happen in a single view change, without
	// any period in which there's no backup.
	fmt.Printf("Test: Idle third server becomes backup if primary fails ...\n")

	{
		// 这句只是为了得到当前的 Viewnum 版本号
		vx, _ := ck2.Get()
		ck2.Ping(vx.Viewnum)
		for i := 0; i < DeadPings*2; i++ {
			ck3.Ping(0)
			v, _ := ck1.Ping(vx.Viewnum)
			if v.Primary == ck1.me && v.Backup == ck3.me {
				break
			}
			vx = v
			time.Sleep(PingInterval)
		}
		// 注意到，这里其实有两个改动，一个是 ck1 成为 Primary
		// 一个是 ck3 成为 Backup ，但是要求 Viewnum 只增加 1
		// 也就是在发现原 Primary ck2 超时的那个 tick 内，需要同时完成两件事
		// 提升 Backup ck1 肯定是没有问题的，这就要求同一 tick 内已经知道还存在 idle 服务器 ck3
		check(t, ck1, ck1.me, ck3.me, vx.Viewnum+1)
	}
	fmt.Printf("  ... Passed\n")

	// kill and immediately restart the primary -- does viewservice
	// conclude primary is down even though it's pinging?
	fmt.Printf("Test: Restarted primary treated as dead ...\n")

	{
		vx, _ := ck1.Get()
		ck1.Ping(vx.Viewnum)
		for i := 0; i < DeadPings*2; i++ {
			// ck1 的 Viewnum 设置为 0，被认为是重启，丢掉了历史版本，故此不能再做 Primary
			ck1.Ping(0)
			ck3.Ping(vx.Viewnum)
			v, _ := ck3.Get()
			if v.Primary != ck1.me {
				break
			}
			time.Sleep(PingInterval)
		}
		vy, _ := ck3.Get()
		if vy.Primary != ck3.me {
			t.Fatalf("expected primary=%v, got %v\n", ck3.me, vy.Primary)
		}
	}
	fmt.Printf("  ... Passed\n")

	fmt.Printf("Test: Dead backup is removed from view ...\n")

	// set up a view with just 3 as primary,
	// to prepare for the next test.
	{
		for i := 0; i < DeadPings*3; i++ {
			vx, _ := ck3.Get()
			ck3.Ping(vx.Viewnum)
			time.Sleep(PingInterval)
		}
		v, _ := ck3.Get()
		if v.Primary != ck3.me || v.Backup != "" {
			t.Fatalf("wrong primary or backup")
		}
	}
	fmt.Printf("  ... Passed\n")

	// does viewserver wait for ack of previous view before
	// starting the next one?
	fmt.Printf("Test: Viewserver waits for primary to ack view ...\n")

	{
		// set up p=ck3 b=ck1, but
		// but do not ack
		vx, _ := ck1.Get()
		for i := 0; i < DeadPings*3; i++ {
			ck1.Ping(0)
			ck3.Ping(vx.Viewnum)
			v, _ := ck1.Get()
			if v.Viewnum > vx.Viewnum {
				// 注意，此时发生在 ck3.Ping 之后，当时 Primary ck3 使用 vx.Viewnum Ping
				// 确认了 ck1 成为 backup 的更新，于是 ViewServer.Viewnum ++
				// 接着 ck1.Get() 获得最新的 Viewnum，接着跳出
				break
			}
			time.Sleep(PingInterval)
		}
		// 接着使用 ck1 去 check；注意，到此时，ck3 仍然还未再次 Ping
		// 就是说 ck3 此时的 View 是落后一个版本的，ck3 从未更新最新的 ViewServer.Viewnum
		check(t, ck1, ck3.me, ck1.me, vx.Viewnum+1)
		vy, _ := ck1.Get()
		// ck3 is the primary, but it never acked.
		// let ck3 die. check that ck1 is not promoted.
		for i := 0; i < DeadPings*3; i++ {
			// ck3 不再 ping，超时，primary 失败
			// 按 case 3. 似乎应该提升 ck1 为 Primary，但是由于原 Primary ck3 从未更新 ack 过最新的View
			// 故此，按设计，这次提升不能生效
			v, _ := ck1.Ping(vy.Viewnum)
			if v.Viewnum > vy.Viewnum {
				break
			}
			time.Sleep(PingInterval)
		}
		// 注意，这个 check 是使用 ck2 来检验的；如果仍然是使用 ck1 来检验，会得到不同的结果么？？？
		// 答案是不会，因为 check 内部使用的是 GET，和发起请求者无关，结果是一致的
		check(t, ck2, ck3.me, ck1.me, vy.Viewnum)
	}
	fmt.Printf("  ... Passed\n")

	// if old servers die, check that a new (uninitialized) server
	// cannot take over.
	fmt.Printf("Test: Uninitialized server can't become primary ...\n")

	{
		for i := 0; i < DeadPings*2; i++ {
			v, _ := ck1.Get()
			ck1.Ping(v.Viewnum)
			ck2.Ping(0)
			ck3.Ping(v.Viewnum)
			time.Sleep(PingInterval)
		}
		for i := 0; i < DeadPings*2; i++ {
			ck2.Ping(0)
			time.Sleep(PingInterval)
		}
		vz, _ := ck2.Get()
		if vz.Primary == ck2.me {
			t.Fatalf("uninitialized backup promoted to primary")
		}
	}
	fmt.Printf("  ... Passed\n")

	vs.Kill()
}
