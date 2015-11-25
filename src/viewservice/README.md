View Service - lab.2 Part A
==============================
http://nil.csail.mit.edu/6.824/2015/labs/lab-2.html
http://nil.csail.mit.edu/6.824/2015/notes/l-rpc.txt
http://nil.csail.mit.edu/6.824/2015/notes/l-rpc.go

###Lab2 overview
**Design ideas**
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


