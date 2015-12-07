PbService - lab.2 Part B
============================
http://nil.csail.mit.edu/6.824/2015/labs/lab-2.html

### PbService Requirements
Your key/value service should continue operating correctly as long as there has never been a time at which no server was alive.

Correct operation means that calls to Clerk.Get(k) return the latest value set by a successful call to Clerk.Put(k,v)
or Clerk.Append(k,v), or an empty string if the key has never seen either.

All operations should provide at-most-once semantics (lec02/l-rpc.txt).

You should assume that the viewservice never halts or crashes.

A danger: suppose in some view S1 is the primary; the viewservice changes views so that S2 is the primary;
but S1 hasn't yet heard about the new view and thinks it is still primary.
Then some clients might talk to S1, and others talk to S2, and not see each others' Put()s.

 server that isn't the active primary should either not respond to clients, or respond with an error:
 it should set GetReply.Err or PutReply.Err to something other than OK.

 Clerk.Get(), Clerk.Put(), and Clerk.Append() should only return when they have completed the operation.
 That is, Put()/Append() should keep trying until they have updated the key/value database,
 and Clerk.Get() should keep trying until it has retrieved the current value for the key (if any).

Server must filter out the duplicate RPCs that these client re-tries will generate to ensure at-most-once semantics.

Your primary should forward just the arguments to each Append() to the backup; not the resulting value, which might be large.


### 已实现部分研读
**common.go**
kv-store 服务可能的返回包括：OK / ErrNoKey / ErrWrongServer，类型为 Err (alias for string)
Put 接口参数为 Key/Value，返回 Err
Get 接口参数为 Key，返回为 Err, Value

**client.go**
Clerk 结构内嵌一个 viewservice.Clerk，这是因为 Client 也要向 ViewServer 发 Get 请求，以获取当前服务器 (主要是 Primary) 的设置
提供 MakeClerk 函数来实例化 Clerk (也就是 Client)
提供了一个随机数生成函数 nrand()
提供了请求调用函数 call 用于发起 rpc 请求
需要实现的请求包括 Get/PutAppend/Put/Append

**server.go**
PBServer 结构内嵌一个 viewservice.Clerk，用于向 ViewServer 发 Ping，以向 ViewServer 更新 kv-store 服务器的状况
内嵌 dead / unreliable 变量以及一些辅助函数，用于测试时模拟停止、失去响应等各种情况
需要实现以下接口以响应请求，包括 Get/PutAppend
需要实现 tick 也就是周期性的任务(向 ViewServer 发送心跳 Ping 请求)
提供了 StartServer 函数的框架，包括：
    初始化 viewservice.Clerk 实例
    注册自身的 RPC 服务接口
    go routine 用于接收 RPC 请求，处理请求，并模拟失去响应的现象
    go routine 用于周期调用 tick 函数，保持心跳

看到，无论是 kv-store 的 Server 还是 Client 都会维护一个 ViewServer 的 Clerk
Server 用于向 ViewServer 通知本身的状态，以及接收系统的最新状态
Client 只用于更新系统的最新状态，以便可以知道向谁发送 kv-store 请求

