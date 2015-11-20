MapReduce
===========

###单机单进程模式
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

