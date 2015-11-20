package mapreduce

import "fmt"
import "os"
import "log"
import "strconv"
import "encoding/json"
import "sort"
import "container/list"
import "bufio"

// Map and Reduce deal with <key, value> pairs:
type KeyValue struct {
	Key   string
	Value string
}

func TestMap(fileName string, fileout string, Map func(string) *list.List) {
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatal("Testmap: ", err)
	}
	fi, err := file.Stat()
	if err != nil {
		log.Fatal("Testmap: ", err)
	}
	size := fi.Size()
	fmt.Printf("Testmap: read split %s %d\n", name, size)
	b := make([]byte, size)
	_, err = file.Read(b)
	if err != nil {
		log.Fatal("Testmap: ", err)
	}
	file.Close()
	res := Map(string(b))
	file, err = os.Create(fileout)
	if err != nil {
		log.Fatal("Testmap: create ", err)
	}
	enc := json.NewEncoder(file)
	for e := res.Front(); e != nil; e = e.Next() {
		kv := e.Value.(KeyValue)
		err := enc.Encode(&kv)
		if err != nil {
			log.Fatal("Testmap: marshall ", err)
		}
	}
	file.Close()
}

func TestReduce(fileName string, Reduce func(string, *list.List) string) {
	kvs := make(map[string]*list.List)
	file, err := os.Opean(fileName)
	if err != nil {
		log.Fatal("TestReduce: ", err)
	}
	dec := json.NewDecoder(file)
	for {
		var kv KeyValue
		err = dec.Decode(&kv)
		if err != nil {
			break
		}
		_, ok := kvs[kv.Key]
		if !ok {
			kvs[kv.Key] = list.New()
		}
		kvs[kv.Key].PushBack(kv.Value)
	}
	file.Close()

	var keys []string
	for k := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		res := Reduce(k, kvs[k])
		fmt.Printf("key: %s  value: %d", k, res)
	}
}

func Map(value string) *list.List {
}

func Reduce(key string, values *list.List) int64 {
}

func RunTest(file string, Map func(string) *list.List, Reduce func(string, *list.List) string) {
	TestMap(file, "mapresultoffile", Map)
	TestReduce("mapresultoffile", Reduce)
}

func main() {
	if len(os.Args) != 4 {
		fmt.Printf("%s: see usage comments in file\n", os.Args[0])
	} else if os.Args[1] == "master" {
		if os.Args[3] == "sequential" {
			mapreduce.RunSingle(5, 3, os.Args[2], Map, Reduce)
		} else {
			mr := mapreduce.MakeMapReduce(5, 3, os.Args[2], os.Args[3])
			// Wait until MR is done
			<-mr.DoneChannel
		}
	} else {
		mapreduce.RunWorker(os.Args[2], os.Args[3], Map, Reduce, 100)
	}
}
