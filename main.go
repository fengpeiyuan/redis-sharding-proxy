package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	masterPort int
	masterHost string
	proxyPort  int
	proxyHost  string
	slavePort int
	slaveHost string
	connectionStr string
	slaveHostPort string
	keyRegexp  *regexp.Regexp
)

const (
	bufSize       = 16384
	channelBuffer = 100
)

type redisCommand struct {
	raw      []byte
	command  []string
	reply    string
	bulkSize int64
}



func readRedisCommand(reader *bufio.Reader) (*redisCommand, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("Failed to read command: %v", err)
	}

	if header == "\n" || header == "\r\n" {
		// empty command
		return &redisCommand{raw: []byte(header)}, nil
	}

	if strings.HasPrefix(header, "+") {
		return &redisCommand{raw: []byte(header), reply: strings.TrimSpace(header[1:])}, nil
	}

	if strings.HasPrefix(header, "$") {
		bulkSize, err := strconv.ParseInt(strings.TrimSpace(header[1:]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Unable to decode bulk size: %v", err)
		}
		return &redisCommand{raw: []byte(header), bulkSize: bulkSize}, nil
	}

	if strings.HasPrefix(header, "*") {
		cmdSize, err := strconv.Atoi(strings.TrimSpace(header[1:]))
		if err != nil {
			return nil, fmt.Errorf("Unable to parse command length: %v", err)
		}

		result := &redisCommand{raw: []byte(header), command: make([]string, cmdSize)}

		for i := range result.command {
			header, err = reader.ReadString('\n')
			if !strings.HasPrefix(header, "$") || err != nil {
				return nil, fmt.Errorf("Failed to read command: %v", err)
			}

			result.raw = append(result.raw, []byte(header)...)

			argSize, err := strconv.Atoi(strings.TrimSpace(header[1:]))
			if err != nil {
				return nil, fmt.Errorf("Unable to parse argument length: %v", err)
			}

			argument := make([]byte, argSize)
			_, err = io.ReadFull(reader, argument)
			if err != nil {
				return nil, fmt.Errorf("Failed to read argument: %v", err)
			}

			result.raw = append(result.raw, argument...)

			header, err = reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("Failed to read argument: %v", err)
			}

			result.raw = append(result.raw, []byte(header)...)

			result.command[i] = string(argument)
		}

		return result, nil
	}

	return &redisCommand{raw: []byte(header), command: []string{strings.TrimSpace(header)}}, nil
}

// Goroutine that handles writing commands to master
func masterWriter(conn net.Conn, masterchannel <-chan []byte) {
	defer conn.Close()

	for data := range masterchannel {
		_, err := conn.Write(data)
		if err != nil {
			log.Printf("Failed to write data to master: %v\n", err)
			return
		}
	}
}

// Connect to master, request replication and filter it
func masterConnection(slavechannel chan<- []byte, masterchannel <-chan []byte) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", masterHost, masterPort))
	if err != nil {
		log.Printf("Failed to connect to master: %v\n", err)
		return
	}

	defer conn.Close()
	go masterWriter(conn, masterchannel)

	reader := bufio.NewReaderSize(conn, bufSize)

	for {
		command, err := readRedisCommand(reader)
		if err != nil {
			log.Printf("Error while reading from master: %v\n", err)
			return
		}

		if command.reply != "" || command.command == nil && command.bulkSize == 0 {
			// passthrough reply & empty command
			slavechannel <- command.raw
			slavechannel <- nil
		} else if len(command.command) == 1 && command.command[0] == "PING" {
			log.Println("Got PING from master")
			slavechannel <- command.raw
			slavechannel <- nil
		} else if command.bulkSize > 0 {
			log.Printf("RDB size: %d\n", command.bulkSize)
			slavechannel <- command.raw
			err = FilterRDB(reader, slavechannel, shardingIsHit, command.bulkSize)
			if err != nil {
				log.Printf("Unable to read RDB: %v\n", err)
				return
			}
			log.Println("RDB filtering finished, filtering commands...")
		} else {
			if len(command.command) >= 2 && shardingIsHit(command.command[1]) == false {
				continue
			}
			slavechannel <- command.raw
			slavechannel <- nil
		}

	}
}

// Goroutine that handles writing data back to slave
func slaveWriter(conn net.Conn, slavechannel <-chan []byte) {
	writer := bufio.NewWriterSize(conn, bufSize)

	for data := range slavechannel {
		var err error

		if data == nil {
			err = writer.Flush()
		} else {
			_, err = writer.Write(data)
		}

		if err != nil {
			log.Printf("Failed to write data to slave: %v\n", err)
			return
		}
	}
}

// Read commands from slave
func slaveReader(conn net.Conn) {
	defer conn.Close()

	log.Print("Slave connection established from ", conn.RemoteAddr().String())

	reader := bufio.NewReaderSize(conn, bufSize)

	// channel for writing to slave
	slavechannel := make(chan []byte, channelBuffer)
	defer close(slavechannel)

	// channel for writing to master
	masterchannel := make(chan []byte, channelBuffer)
	defer close(masterchannel)

	go slaveWriter(conn, slavechannel)
	go masterConnection(slavechannel, masterchannel)

	for {
		command, err := readRedisCommand(reader)
		if err != nil {
			log.Printf("Error while reading from slave: %v\n", err)
			return
		}

		if command.reply != "" || command.command == nil && command.bulkSize == 0 {
			// passthrough reply & empty command
			masterchannel <- command.raw
		} else if len(command.command) == 1 && command.command[0] == "PING" {
			log.Println("Got PING from slave")

			masterchannel <- command.raw
		} else if len(command.command) == 1 && command.command[0] == "SYNC" {
			log.Println("Starting SYNC")

			masterchannel <- command.raw
		} else if len(command.command) == 3 && command.command[0] == "REPLCONF" && command.command[1] == "ACK" {
			log.Println("Got ACK from slave")

			masterchannel <- command.raw
		} else {
			// unknown command
			slavechannel <- []byte("+ERR unknown command\r\n")
			slavechannel <- nil
		}
	}
}

//bin/redis-sharding-proxy -master-host=10.209.37.78 -master-port=10001 -proxy-host=10.209.37.78 -proxy-port=10000 -slave-host=10.209.37.78 -slave-port=10002 10.209.37.78:10002,10.209.37.78:10003    

//./redis-sharding-proxy -master-host=10.209.37.78 -master-port=10000 -proxy-host=10.209.37.78 -proxy-port=11001 -slave-host=10.209.37.78 -slave-port=10001 10.209.37.78:10001,10.209.37.78:10002,10.209.37.78:10003,10.209.37.78:10004,10.209.37.78:10005

//./redis-sharding-proxy -master-host=10.209.37.78 -master-port=10000 -proxy-host=10.209.37.78 -proxy-port=11002 -slave-host=10.209.37.78 -slave-port=10002 10.209.37.78:10001,10.209.37.78:10002,10.209.37.78:10003,10.209.37.78:10004,10.209.37.78:10005

//./redis-sharding-proxy -master-host=10.209.37.78 -master-port=10000 -proxy-host=10.209.37.78 -proxy-port=11003 -slave-host=10.209.37.78 -slave-port=10003 10.209.37.78:10001,10.209.37.78:10002,10.209.37.78:10003,10.209.37.78:10004,10.209.37.78:10005

//./redis-sharding-proxy -master-host=10.209.37.78 -master-port=10000 -proxy-host=10.209.37.78 -proxy-port=11004 -slave-host=10.209.37.78 -slave-port=10004 10.209.37.78:10001,10.209.37.78:10002,10.209.37.78:10003,10.209.37.78:10004,10.209.37.78:10005

//./redis-sharding-proxy -master-host=10.209.37.78 -master-port=10000 -proxy-host=10.209.37.78 -proxy-port=11005 -slave-host=10.209.37.78 -slave-port=10005 10.209.37.78:10001,10.209.37.78:10002,10.209.37.78:10003,10.209.37.78:10004,10.209.37.78:10005

func main() {
	flag.StringVar(&masterHost, "master-host", "localhost", "Master Redis host.")
	flag.IntVar(&masterPort, "master-port", 6379, "Master Redis port.")
	flag.StringVar(&proxyHost, "proxy-host", "", "Proxy listening interface, waiting for slave to connect.")
	flag.IntVar(&proxyPort, "proxy-port", 6380, "Proxy port waiting for slave to connect.")
	flag.StringVar(&slaveHost, "slave-host", "", "Slave redis host.")
	flag.IntVar(&slavePort, "slave-port", 6381, "Slave redis port.")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "Please specify connection string, like: host1:port1,host2:port2 ... hostn:portn .")
		os.Exit(1)
	}

	var err error
	connectionStr = flag.Arg(0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Wrong format of connection string: %v", err)
		os.Exit(1)
	}

	slaveHostPort = slaveHost + ":" +strconv.Itoa(slavePort)

	log.Printf("Redis Master %s:%d\n", masterHost, masterPort)
	log.Printf("Redis Rehashing Proxy %s:%d\n", proxyHost, proxyPort)
	log.Printf("Redis Slave %s:%d\n", slaveHost, slavePort)
	log.Printf("Redis connection string %s\n", connectionStr)

	// prepare to initial connection string
	shardingInit()

	//test some key
	//log.Printf("key_111:")
	//shardingIsHit("key_111")
	//log.Printf("key_7190:")
	//shardingIsHit("key_7190")
	//log.Printf("key_8834:")
	//shardingIsHit("key_8834")
	//log.Printf("key_10133:")
	//shardingIsHit("key_10133")
	//log.Printf("key_10153:")
	//shardingIsHit("key_10153")
	//os.Exit(0)

	// listen for incoming connection from Redis slave
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", proxyHost, proxyPort))
	if err != nil {
		log.Fatalf("Unable to listen: %v\n", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Unable to accept: %v\n", err)
			continue
		}

		go slaveReader(conn)
	}
}
