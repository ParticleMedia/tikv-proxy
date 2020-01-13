// +build ignore

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/ParticleMedia/tikv-proxy/common"
	"github.com/ParticleMedia/tikv-proxy/server"
	"github.com/golang/glog"
	"github.com/pingcap/tidb/store/tikv"
	"os"
)

const (
	DEFAULT_CONFIG_PATH = "../conf/tikv_proxy.yaml"
)

var conf = flag.String("conf", DEFAULT_CONFIG_PATH, "path of config file")
var prefix = flag.String("prefix", "", "scan key prefix")
var batch = flag.Uint("batch", 1000, "key count for one scan batch")

func checkArgs() {
	if len(*conf) == 0 {
		fmt.Fprint(os.Stderr, "config file path is needed!\n")
		os.Exit(255)
	}
	if len(*prefix) == 0 {
		fmt.Fprint(os.Stderr, "scan key prefix is needed!\n")
		os.Exit(255)
	}
	if *batch < 1 {
		*batch = 1
	}
}

func loadConfig() {
	// config
	confErr := common.LoadConfig(*conf)
	if confErr != nil {
		fmt.Fprintf(os.Stderr, "failed to config: %+v\n", confErr)
		os.Exit(255)
	}
	if !common.ProxyConfig.Check() {
		fmt.Fprintln(os.Stderr, "check config failed")
		os.Exit(255)
	}
}

func doScan(client *tikv.RawKVClient, prefix string, batch int, handler func([][]byte, [][]byte)()) error {
	if client == nil {
		return errors.New("invalid tikv client")
	}

	var keys [][]byte = nil
	var values [][]byte = nil
	var hasMore bool = true
	var err error = nil
	bytePrefix := []byte(prefix)
	startKey := bytePrefix

	for {
		glog.V(16).Infof("start scan from key: %s batch: %d", startKey, batch)
		keys, values, err = client.Scan(startKey, batch)
		if err != nil {
			return err
		}

		if keys == nil || values == nil || len(keys) == 0 || len(values) == 0 {
			break
		}

		for i, key := range keys {
			if !bytes.HasPrefix(key, bytePrefix) {
				keys = keys[0:i]
				values = values[0:i]
				hasMore = false
				break
			}
		}
		handler(keys, values)
		if !hasMore {
			break
		}
		startKey = append(keys[len(keys) - 1], 0)
	}

	return nil
}

func handler(keys [][]byte, values [][]byte) {
	for _, k := range keys {
		fmt.Println(string(k))
	}
}

func main() {
	flag.Parse()
	checkArgs()
	loadConfig()

	client, err := server.BuildTikvClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build tikv client with error: %+v\n", err)
		os.Exit(255)
	}

    err = doScan(client, *prefix, int(*batch), handler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan tikv with error: %+v\n", err)
		os.Exit(255)
	}
}
