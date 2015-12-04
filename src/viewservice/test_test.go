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
		// 第一次 Ping，升级为 Primary, primaryAck = 0 但是 view.Viewnum = 1
		view, _ := ck1.Ping(0)
		// 故此，这里跳出，其实不跳出也没关系，因为即使后面再次 Ping，
		// 只会调用 PromoteBackup() ，因为没有 Backup 故此什么也不做
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
			// ck1 Ping 1，按设计，就是说 Primary 告诉 ViewServer 更改已经同步，此时 Acked() 成立
			ck1.Ping(1)
			// 第一次调用即把 ck2 设置为 Backup
			view, _ := ck2.Ping(0)
			// 成立，跳出
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
		// 同样，保持 ck1 也就是 Primary 同步， Acked() 成立
		ck1.Ping(2)
		vx, _ := ck2.Ping(2)
		for i := 0; i < DeadPings*2; i++ {
			// ck2 保持同步 Viewnum 的 Ping，故此只更新 backupAck 而已
			v, _ := ck2.Ping(vx.Viewnum)
			// 由于 ck1 同步之后不再 Ping，故此会超时，而且 Acked() 成立，故此会调用 PromoteBackup()
			// 下面的条件成立，跳出；同时 primaryAck 重置为 0，Acked() 不再成立
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
		// 先取当前 ViewServer 的 view.Viewnum 值
		vx, _ := ck2.Get()
		// ck2 也就是 Primary 来同步 Ping，会设置 primaryAck = vx.Viewnum ， Acked() 再次成立
		ck2.Ping(vx.Viewnum)
		for i := 0; i < DeadPings*2; i++ {
			// 这时，相当于上面的 case.2 ，在 Acked() 成立的前提下，设置 Backup，view.Viewnum 增1
			ck1.Ping(0)
			// ck2 Primary 仍然保持原来的 view.Viewnum 来 Ping，保持 Primary 身份，但是不再 Acked()
			v, _ := ck2.Ping(vx.Viewnum)
			// 成功跳出，跳出时是 Acked() 不成立的状态
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
	// 这个 case 比较复杂，需仔细查看 ！！！
	fmt.Printf("Test: Idle third server becomes backup if primary fails ...\n")

	{
		// 这句只是为了得到当前的 Viewnum 版本号
		vx, _ := ck2.Get()
		// 这句让 Primary keep up with 当前的 Viewnum，Acked() 再次成立
		ck2.Ping(vx.Viewnum)
		for i := 0; i < DeadPings*2; i++ {
			// 第一轮 Primary 超时后的循环：
			// Acked() 成立的条件下，ck2 Primary 超时，故此提升 Bakcup，Backup 被提升后，primaryAck 重置 0，不再 Acked
			// 虽然先调用的 ck3.Ping(0)，但是在没有 提升 Backup 的时候，Primary & Backup 都是占据的，故此 ck3 只能是空闲
			// 故此，一定是 提升 Backup 先发生，view.Viewnum 增 1
			// 第三轮 Primary 超时后的循环：
			// 由于第二轮会重新 Acked() 成立，而且 Backup == ""，故此第三轮时会设置 Backup，且 Acked() 不再成立
			ck3.Ping(0)
			//  第二轮 Primary 超时后的循环，到此时，Acked() 再次满足，因为第一轮中把 vx 设置为 view 的最新状态
			v, _ := ck1.Ping(vx.Viewnum)
			// 第一轮 Primary 超时后的循环中不满足条件
			// 第三轮 Primary 超时后的循环中满足条件，跳出，跳出时 Acked() 不成立
			if v.Primary == ck1.me && v.Backup == ck3.me {
				break
			}
			// 第一轮 Primary 超时后的循环：
			// 这里把 vx 赋值为 v，也就是提升 Backup 之后的版本
			// 这样，下次循环到 ck1.Ping(vx.Viewnum)，就满足 Acked() 成立了
			vx = v
			time.Sleep(PingInterval)
		}
		// 注意到，这里其实有两个改动，一个是 ck1 成为 Primary
		// 一个是 ck3 成为 Backup ，但是这里要求 Viewnum 只增加 1？
		// 其实设这样的，中间第一轮 Primary 超时后的循环中，会更新 vx 为第一个改动发生后的 view
		// 故此，这里了的 vx 已经不是 case 最开始时的初始 view 了
		check(t, ck1, ck1.me, ck3.me, vx.Viewnum+1)
	}
	fmt.Printf("  ... Passed\n")

	// kill and immediately restart the primary -- does viewservice
	// conclude primary is down even though it's pinging?
	fmt.Printf("Test: Restarted primary treated as dead ...\n")

	{
		// 同样使用这两句话，把当前的 Primary 和 ViewServer 同步，Acked() 满足
		vx, _ := ck1.Get()
		ck1.Ping(vx.Viewnum)
		for i := 0; i < DeadPings*2; i++ {
			// ck1 的 Viewnum 设置为 0，被认为是重启，丢掉了历史版本，故此不能再做 Primary
			// 此时调用提升 Backup, Viewnum 也会增 1，并把 Backup 设置为 ""
			ck1.Ping(0)
			// ck3 此时已经是 Primary 了，但是在使用老的 vx.Viewnum 来 Ping，故此保持 Acked() 不成立
			ck3.Ping(vx.Viewnum)
			// 这个 v 是最新的 view
			v, _ := ck3.Get()
			// 满足，跳出，此时 Acked() 不成立，而且 Backup = ""
			if v.Primary != ck1.me {
				break
			}
			time.Sleep(PingInterval)
		}
		// vy 是最新的 view
		vy, _ := ck3.Get()
		// 最新 view 的 Primary 就是 ck3，此时 Acked() 不成立，而且  Backup = ""
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
			// 同样，两个语句保持 ck3 Acked() 成立
			vx, _ := ck3.Get()
			ck3.Ping(vx.Viewnum)
			time.Sleep(PingInterval)
		}
		// v 是最新 view，且 Acked() 成立，Backup = ""，满足 test 条件
		v, _ := ck3.Get()
		// 因为 ck1 重启 Ping(0) 之后，再也没有 Ping 过，故此不会被设置为 Backup
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
			// 此时，把 ck1 设置为 Backup，view.Viewnum 增1，故此并不 Acked()
			ck1.Ping(0)
			// Primary 仍然使用老的 viewnum Ping， 故此也不会 Acked()，这点很重要
			ck3.Ping(vx.Viewnum)
			v, _ := ck1.Get()
			// 显然，最新的 view 中是 ck1 为 Backup 的状态， viewnum 比 老的 viewnum 多了 1，故此满足
			if v.Viewnum > vx.Viewnum {
				break
			}
			time.Sleep(PingInterval)
		}
		// 最新的 view，显然是满足下面条件，只不过目前 Acked() 不成立而已
		check(t, ck1, ck3.me, ck1.me, vx.Viewnum+1)
		vy, _ := ck1.Get()
		// ck3 is the primary, but it never acked.
		// let ck3 die. check that ck1 is not promoted.
		for i := 0; i < DeadPings*3; i++ {
			// ck3 不再 ping，超时，primary 失败
			// 按 case 3. 似乎应该提升 ck1 为 Primary，但是由于原 Primary ck3 从未更新 ack 过最新的View
			// 故此，按设计，这次提升不可能发生，因为 Acked() 不满足
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
			// 获取最新 view
			v, _ := ck1.Get()
			// ck1 使用最新 v 来 Ping,保持 Backup 状态
			ck1.Ping(v.Viewnum)
			// ck2 Ping(0)，空闲服务器，不发生任何事
			ck2.Ping(0)
			// ck3 用最新的 v 来 Ping，说明 Primary 归来，系统恢复 Acked() 状态
			ck3.Ping(v.Viewnum)
			time.Sleep(PingInterval)
		}
		vs.debug()
		// 此时 ViewServer 恢复为 Acked() 状态，ck1 是 Backup, ck3 是 Primary
		for i := 0; i < DeadPings*2; i++ {
			// 只让 ck2 Ping，也就是说让 ck1 & ck3 超时
			ck2.Ping(0)
			// case 1. Primary ck3 先超时，那么 PromoteBackup，于是 ck1 成为 Primary；问题是 ck1 也不连接，于是不会 Acked，僵死
			// case 2. Backup ck1 先超时，这种情况会不容易发生，因为 server.go::Tick() 中先判断 Primary 在判断 Backup
			// 于是，设置 Backup = ""，并把 view.Viewnum ++，于是比 Primary 高了，而 ck3 不会再连接，于是也不会 Acked，僵死
			time.Sleep(PingInterval)
		}
		vz, _ := ck2.Get()
		vs.debug()
		// 这个用例说明，如果 Acked 的系统，且 Primary & Backup 都在，如果它们同时失联，那么系统一定僵死
		if vz.Primary == ck2.me {
			t.Fatalf("uninitialized backup promoted to primary")
		}
	}
	fmt.Printf("  ... Passed\n")

	vs.Kill()
}
