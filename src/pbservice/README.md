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

