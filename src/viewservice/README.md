View Service - lab.2 Part A
==============================
http://nil.csail.mit.edu/6.824/2015/labs/lab-2.html
http://nil.csail.mit.edu/6.824/2015/notes/l-rpc.txt
http://nil.csail.mit.edu/6.824/2015/notes/l-rpc.go

###Lab2 overview
**Design ideas** one primary and one backup 
In this lab you'll make a key/value service fault-tolerant using a form of primary/backup replication.
In order to ensure that all parties (clients and servers) agree on which server is the primary, and which is the backup, we'll introduce a kind of master server, called the viewservice.
The viewservice monitors whether each available server is alive or dead. If the current primary or backup becomes dead, the viewservice selects a server to replace it.
A client checks with the viewservice to find the current primary. The servers cooperate with the viewservice to ensure that at most one primary is active at a time.
If the primary fails, the viewservice will promote the backup to be primary.
If the backup fails, or is promoted, and there is an idle server available, the viewservice will cause it to be the backup.
The primary will send its complete database to the new backup, and then send subsequent Puts to the backup to ensure that the backup's key/value database remains identical to the primary's.
It turns out the primary must send Gets as well as Puts to the backup (if there is one), and must wait for the backup to reply before responding to the client.
Key/value server will keep the data in memory, not on disk.
One consequence of keeping data only in memory is that if there's no backup, and the primary fails, and then restarts, it cannot then act as primary.
Only RPC may be used for interaction between clients and servers, between different servers, and between different clients.

**Drawback on performance**
The view service itself is vulnerable to failures, since it's not replicated.
The primary and backup must process operations one at a time, limiting their performance.
A recovering server must copy a complete database of key/value pairs from the primary, which will be slow, even if it has almost-up-to-date copy of data.
The servers don't store the key/value database on disk.
If a temporary problem prevents primary to backup communication, the system has only two remedies: change the view to eliminate the backup, or keep trying; neither performs well if such problems are frequent.
If a primary fails before acknowledging the view in which it is primary, the view service cannot make progress---it will spin forever and not perform a view change.

**reference**
Similiar yet better protocal: Chain Replication - http://www.cs.cornell.edu/home/rvr/papers/osdi04.pdf



###Part A. View Service
View = a view number + identities (addr & port) of primary and backup servers.
View Server maintains a series of the Views corresponding the status changes of the servers.
Each key/value server should send a Ping RPC once per *PingInterval* (see viewservice/common.go).
The view service replies to the Ping with a description of the current view.
If the viewservice doesn't receive a Ping from a server for *DeadPings PingIntervals*, the viewservice should consider the server to be dead.
When a server re-starts after a crash, it should send one or more Pings with an argument of zero to inform the view service that it crashed.
The view service proceeds to a new view if the below cases happen：
    - it hasn't received recent Pings from both primary and backup
    - the primary or backup crashed and restarted
    - there is no backup and there is an idle server
View service must NOT change views (i.e., return a different view to callers) until the primary from the current view acknowledges that it is operating in the current view (by sending a Ping with the current view number).
That is, the view service may not proceed from view X to view X+1 if it has not received a Ping(X) from the primary of view X, even if it thinks that something has changed and it should change the view.
The *acknowledgement rule* corresponds the last point of the Drawback section, and the point is to prevent the view service from getting more than one view ahead of the key/value servers.


**common.go**
定义 View:  Viewnum uint + Primary string + Backup  string
定义 Ping 间隔时间 PingInterval = 100 ms
定义失联间隔时间 DeadPings 为 5 个 PingInterval 即 500 ms
PingArg: me 自身的 host/port + 所持有的 Viewnum，用于通知 ViewServer 自己仍然存活
PingReply: View，用于 ViewServer 向请求方返回最新的 View
GetArg: 无参数，用于 client 向 ViewServer 索求最新的 View，由于不会提供调用者信息，故此该接口不会更新 Primary/Backup 的设置
GetReply: View，用于 ViewServer 向请求方返回最新的 View


**client.go**
client 称为 Clerk，维护 me 自身的 host/port + server 即 ViewServer；注意，并不维护 Viewnum
定义了 Rpc 调用函数 call，和 MapReduce 中的是完全一致的，是同步且要等待结果返回的
Clerk 实现方法 Ping，使用函数参数 viewnum 向 ViewServer 发送 Ping 请求，如果失败返回空 View，否则返回 ViewServer 的 reply.View
Clerk 实现方法 Get，向 ViewServer 发送 Get 请求，如果失败返回空 View，否则返回 ViewServer 的 reply.View
Clerk 实现方法 Primary，调用 Get 方法，并返回得到的 View 中的 Primary，也即主服务器
注意，本文件名字叫 client.go ，所谓 client 是指 ViewServer 的 Client，其实本身也可以是 Server，比如 Primary 或者 Backup Server


**server.go**
ViewServer 维护 mu Mutex, l Listener, dead 自身状态, rpccount 接收的 rpc 调用次数 和 me 自身的 host/port
需要实现 Ping 接口，Get 接口
需要实现 tick 函数，用于在每个 PingInterval 间隔里检查是否有服务器变动，并相应调整 View
实现了 Kill 函数，用于向维护的 dead 变量原子写入 1，然后关掉 l Listener
实现了 isdead 函数，原子查询是否所维护的 dead 变量已经非 0
实现了 GetRPCCount 函数，原子查询所维护的 rpccount 变量，表示已接收的 rpc 请求数目
提供了启动函数 StartServer 的框架，需要补全，该函数框架如下：
    new ViewServer 指针，初始化 me 变量
    创建 rpc server，并注册到自身
    监听得到 l 变量，本测试用例是使用 unix domain socket 来实现监听对象的
    启动 goroutine，只要自身没有 dead，就 loop
        Accept 监听地址取出请求的连接 conn，并原子增加 rpccount 变量
        启动新的 goroutine 为 conn 服务
        如果 Accept 出错，则 Kill 自身，设置 dead 为 1
    启动 goroutine，只要自身没有 dead 就 loop
        调用 tick 来更新所监控的 Servers 的信息
        等待 PingInterval 的时间


**test_test.go**
先看一个基本的测试用例函数 check(ck, p, b, n):
给定 ck Clerk 和 p Primary, b Backup 和 n Viewnum
使用 ck 去发起 Get 请求， 得到 view，然后校验，确保：
    view.Primary == p
    view.Backup == b
    view.Viewnum = n
    ck.Primary() == p 也就是说再次发起一个 Get 请求，得到的仍然是 p

测试流程如下：
    使用 StartServe 创建 ViewServer，名为 v
    使用 MakeClerk 创建 3 个 Clerks，名为 ck1，ck2，ck3

    确保 ck1.Primary() 得到的不是 ""，也就是说 ck1 去 Get，此时 v 应该分配新的 Primary 了
    然后以 PingInterval 为间隔多次用 ck1 Ping v，保证返回的 view.Primary 都是 ck1 自身
    确保此时满足 check(ck1, ck1.me, "", 1)  即 Primary 为 ck1，Backup 为空，Viewnum 为 1

    然后以 PingInterval 为间隔多次用 ck1 和 ck2 来 Ping，其中 ck1 使用自己的 viewnum 1 来 Ping(1)， ck2 使用 0 来 Ping(2)
    确保此时返回的 view.Backup 已经设置为 ck2.me，并 check(ck1, ck1.me, ck2.me, 2)

    然后 ck1 不再 Ping, ck2 保持 Ping(2)，并超过 DeadPing 倍时间间隔，于是 ck1 死亡
    确保 check(ck2, ck2.me, "", 3) 也就是 ck2 成为 Primary，没有了 Backup，Viewnum 升到 3

    然后 ck1 恢复 Ping(0)，ck2 也保持 Ping
    确保 check(ck2, ck2.me, ck1.me, 4) 也就是 ck1成为 Backup，Viewnum 升级到 4

    然后 ck2 死亡，ck3 加入
    确保 check(ck1, ck1.me, ck3.me, 5) 也就是 ck1 成为 Primary，ck3成为 Backup

    然后，ck1 和 ck3 保持 Ping，但是 ck1.Ping(0)，模拟在 DeadPing 间隔内重启的 case
    但是此时仍然要更新 View，因为 ck1 没有保持其状态，其 viewnum 变量了 0
    确保 Primary == ck3.me

    然后只让 ck3 保持 Ping，也就是让 Primary = ck3.me, Backup = ""

    然后恢复 ck1.Ping(0) 成为 Backup；注意，过程中先用 ck3.Ping，再用 ck1.Ping(0)
    一旦发现 ck1.Ping 的结果是更新了View，退出循环，故此 ck3 不会再次 Ping
    之后只用 ck1.Ping 不再用 ck3.Ping，就是说 ck3 仍然没有更新最新的 View
    而由于 ck3 是之前的 Primary，我们设计为一旦当前的 Primary 没有更新最新的 view，那么就不更新, ck1 不会成为 Primary
    于是，继续使用 ck1.Ping(新Viewnum) 超过 DeadPing 次
    确保在 ViewServer 发现 ck3 死掉之后，再使用 ck2 发 Get 请求
    于是，check(ck2, ck3.me, ck1.me, 之前版本的 Viewnum)
    确保 ck3 仍然是 Primary, ck1仍然是 Backup, Viewnum 和 ck1 目前的一致
    (如果 ck3 曾经更新过新版本，再死掉，那么 ck1 就会被提升为 Primary；然而 ck3 从未更新过)

    最后，使用 ck1.Ping(v.Viewnum), ck2.Ping(0), ck3.Ping(Viewnum)
    此时，由于 ViewServer 中维护的 Primary 是 ck3，故此复位，一切重回正常
    而后，再让 ck1 & ck3 死掉，同时让 ck2.Ping(0)
    和上一段类似，由于 Primary 已经死掉，无法接收 ack，故此 ViewServer 再度陷入混乱
    ViewServer 保持原来的版本不动，ck2 不会提升为 Primary

核心逻辑就是：ViewServer 维护 Server 的动态，调整 View
但是这个调整后的 View 版本必须要通知 Primary 并得到确认才生效
否则，ViewServer 就无法继续


**server.go 实现**
需要实现 Ping 接口，Get 接口
需要实现 tick 函数，用于在每个 PingInterval 间隔里检查是否有服务器变动，并相应调整 View
补全 StartServer 的框架

实现如下：
ViewServer 维护以下内部成员变量
    - view  -- 当前 View
        - Primary
        - Backup
        - Viewnum
    - primaryAck  -- Primary 服务器的 Viewnum
    - primaryTick -- Primary 服务器的 Tick
    - backupTick  -- Backup  服务器的 Tick
    - currentTick -- ViewServer 自身的 Tick

定义两个重要的概念
    - Acked()  -- Primary_Ack == View.Viewnum
          根据设计，后续的变更必须先等 Primary Acked 完毕
    - PromoteBackup()
        - view.Primary = view.Backup
        - view.Backup = ""
        - view.Viewnum ++
        - primaryAck = 0
        - primaryTick = backupTick
          首先，view.Backup = ""，之后不再继续找空闲服务器来设置 Backup；
          这是因为后面活跃的空闲服务器还是会 Ping 的，在 Ping 里面设置 Backup 即可，不必在这里
          其次， **primaryAck = 0** 很重要，说明目前并不 Acked; 需要等 Primary 再用 View.Viewnum Ping 一次才能 Acked
          最后，使用原 backupTick 来更新 primaryTick

Get() 函数
这个函数比较简单，直接 reply.View = vs.view 即可，不改变内部状态

Tick() 函数
ViewServer 自身的 currentTick ++，表示当前 Tick 数
然后通过 currentTick 来检查 primaryTick && backupTick 是否超时
如果 primaryTick 超时，且当前 Acked()，PromoteBackup()
如果 backupTick 超时，且当前 Acked()，那么 Backup = "" && Viewnum ++
这里要注意，都要确保 Acked 状态，否则不会发生内部状态变更

Ping() 函数，最复杂和最核心的函数
设 Ping 的发起者其名字为 name，其 Viewnum 为 num
那么，**按先后顺序** 做一下 4 个情况的处理：
    - 最优先处理初始情况：
        - 不必考虑 Acked 的情况，直接让请求者为 Primary
        - 提升 Viewnum = 1， 并更新 primaryTick 为 currentTick
        - **让 primaryAck = 0**，也就是当前并不 Acked，直到某时刻 primaryTick 使用 1 来 Ping
    - 如果是 Primary 来 Ping
        - 如果 Primary 使用 0 来 Ping, 说明 Primary 重启状态，那么 PromoteBackup()
          注意，因为是 Primary 的 Ping, 故此 Primary 只是落后了，而不是失联了；不判断 Acked 就可以提升
        - 否则，更新 primaryAck 为 num，并更新 primaryTick
    - 如果没有 Backup 且当前 Acked，注意 **优先级低于上面两个，故此 Ping 的发起者必然不是 Primary**
        - 设来访者为 Backup
    - 如果是 Backup 来 Ping
        - 如果 num == 0 且 Acked, 说明 Backup 重启后重连，重新设置 Backup 一遍
        - 否则，直接更新 backupTick 即可
最后，设置 reply.View = vs.view 并返回
注意，前两个 Case 要么是没有 Primary 要么是 Primary 重启后重连
那么，这两个 Case 不需要判断 Acked
其他的两个 Case 以及 Tick() 函数中的两个超时 Case，都需要判断 Acked，
确保 Primary 和 ViewServer 同步后才能做后续更改；
尤其注意，即使 Primary 失联，也要求失联前是同步的，否则系统不会改变

以上三个函数都需要使用 mu.Lock && mu.Unlock 上锁和解锁

以上的逻辑，是如何满足测试用例的，请参考 test_test.go

