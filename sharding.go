package main
	
import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"redis-sharding-proxy/gods/maps/treemap"
	"redis-sharding-proxy/gods/utils"
)

var (
	tm *treemap.Map
)

func shardingInit() {
	tm = treemap.NewWith(utils.UInt64Comparator)
	hostPortArr := strings.Split(connectionStr,",")
	for n := 0; n < len(hostPortArr); n++ {
		hostPort := hostPortArr[n]
		fmt.Errorf("hostPort: %s\n", hostPort)
		for i := 0; i < 160; i++ {
			vNode := "SHARD-"+strconv.Itoa(n)+"-NODE-"+strconv.Itoa(i)
			vNodeHash := MurmurHash64A([]byte(vNode),0x1234ABCD)
			//log.Printf("vNode:%s,vNodeHash: %d\n", vNode,vNodeHash)
			tm.Put(vNodeHash,hostPort)
		}
	}
}

func shardingIsHit(key string) bool { 
	keyHash := MurmurHash64A([]byte(key),0x1234ABCD)

	// find tail 
	findKeyHash,findHostPort := tm.Find(func(key interface{}, value interface{}) bool {
		return key.(uint64) >= keyHash
	})
	// if last return 1st
	if findKeyHash == nil {
		findKeyHash,findHostPort = tm.Min()
	}

	hostPortStr,ok := findHostPort.(string)
	//log.Printf("keyHash:%d,findKeyHash:%d,hostPort:%s \n",keyHash,findKeyHash,hostPortStr)

	if ok {
		if strings.Compare(hostPortStr,slaveHostPort) == 0 {
			//log.Printf("hit,hostPortStr:%s, slaveHostPort:%s \n",hostPortStr,slaveHostPort)
			return true
		}
	}
	
	
	return false

}