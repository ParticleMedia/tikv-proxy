// +build ignore

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"github.com/ParticleMedia/tikv-proxy/common"
	"github.com/ParticleMedia/tikv-proxy/server"
	"github.com/pingcap/tidb/store/tikv"
	"os"
	"strings"
)

const (
	DEFAULT_CONFIG_PATH = "../conf/tikv_proxy.yaml"
)

var conf = flag.String("conf", DEFAULT_CONFIG_PATH, "path of config file")
var batch = flag.Uint("batch", 1000, "key count for one scan batch")

func checkArgs() {
	if len(*conf) == 0 {
		fmt.Fprint(os.Stderr, "config file path is needed!\n")
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

func doDelete(client *tikv.RawKVClient, keys [][]byte) error {
	if client == nil {
		return errors.New("invalid tikv client")
	}

	err := client.BatchDelete(keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete tikv with error : %+v\n", err)
	}
	return err
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

	keys := make([][]byte, 0, *batch)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}

		keys = append(keys, []byte(line))
		if len(keys) >= int(*batch) {
			doDelete(client, keys)
			keys = make([][]byte, 0, *batch)
		}
	}

	if len(keys) > 0 {
		doDelete(client, keys)
	}
}
