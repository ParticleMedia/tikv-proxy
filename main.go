package main

import (
    "flag"
    "fmt"
    "github.com/NYTimes/gziphandler"
    "github.com/ParticleMedia/tikv-proxy/server"
    "github.com/golang/glog"
    "github.com/ParticleMedia/tikv-proxy/common"
    "github.com/rcrowley/go-metrics"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "runtime"
    "sync"
    "syscall"
    "time"
)

const (
    defaultConfigPath = "../conf/tikv_proxy.yaml"
)

var configFile = flag.String("conf",defaultConfigPath,"path of config")
var metricsToTsdb = flag.Bool("metrics_to_tsdb",false,"push metrics to tsdb")
var metricsPrefix = flag.String("metrics_prefix", "tikv","metrics prefix for opentsdb")

var proxyServer *server.ProxyServer = nil
var httpServer *http.Server = nil
var stopOnce = &sync.Once{}

func initGlobalResources() {
    // config
    confErr := common.LoadConfig(*configFile)
    if confErr != nil {
        glog.Fatalf("failed to config: %+v", confErr)
    }
    if !common.ProxyConfig.Check() {
        glog.Fatalf("check config failed")
    }
    glog.Infof("Load config file: %+v success", *configFile)
    glog.V(16).Infof("config content: %+v", *common.ProxyConfig)

    // emit metrics
    if *metricsToTsdb {
        addr, tsdbErr := net.ResolveTCPAddr("tcp", common.ProxyConfig.Tsdb.Addr)
        if tsdbErr != nil {
            glog.Fatalf("resolve tsdb address %s error: %+v", common.ProxyConfig.Tsdb.Addr, tsdbErr)
        }
        go metrics.OpenTSDB(metrics.DefaultRegistry, time.Duration(common.ProxyConfig.Tsdb.Duration) * time.Minute, *metricsPrefix, addr)
    }
    go metrics.Log(metrics.DefaultRegistry, 1 * time.Minute, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))
    metrics.GetOrRegister("goroutine.count", metrics.NewFunctionalGauge(func() int64 { return int64(runtime.NumGoroutine()) }))
}

func releaseGlobalResources() {
    if httpServer != nil {
        httpServer.Close()
        httpServer = nil
        glog.Info("Http server closed!")
    }
    if proxyServer != nil {
        proxyServer.Close()
        proxyServer = nil
    }

    metrics.DefaultRegistry.UnregisterAll()
}

func handleSignal(c <-chan os.Signal) {
    for s := range c {
        switch s {
        case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
            glog.Infof("Exti with signal: %v", s)
            stopOnce.Do(releaseGlobalResources)
            glog.Flush()
            os.Exit(0)
        }
    }
}

func main() {
    flag.Parse()
    defer func() { // 必须要先声明defer，否则不能捕获到panic异常
        if err := recover(); err != nil {
            fmt.Println(err) // 这里的err其实就是panic传入的内容
        }
    }()
    defer glog.Flush()

    initGlobalResources()
    defer stopOnce.Do(releaseGlobalResources)

    //创建监听退出chan
    c := make(chan os.Signal)
    //监听指定信号 ctrl+c kill
    signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go handleSignal(c)

    addr := fmt.Sprintf("0.0.0.0:%d", common.ProxyConfig.ListenPort)
    lis, err := net.Listen("tcp", addr)
    if err != nil {
        glog.Fatalf("failed to listen: %+v", err)
    }
    glog.Infof("Listen port: %+v", common.ProxyConfig.ListenPort)

    mux := http.NewServeMux()
    proxyServer, err = server.NewProxyServer(mux)
    if (err != nil) {
        glog.Fatalf("failed to init proxy server: %+v", err)
    }

    httpServer = &http.Server{
        Addr: addr,
        Handler: gziphandler.GzipHandler(mux),
        WriteTimeout: time.Duration(common.ProxyConfig.Server.WriteTimeout) * time.Millisecond,
        ReadHeaderTimeout: time.Duration(common.ProxyConfig.Server.ReadHeaderTimeout) * time.Millisecond,
        ReadTimeout: time.Duration(common.ProxyConfig.Server.ReadTimeout) * time.Millisecond,
        IdleTimeout: time.Duration(common.ProxyConfig.Server.IdleTimeout) * time.Minute,
    }
    err = httpServer.Serve(lis)
    if err != nil {
        glog.Fatalf("failed to serve http: %+v", err)
    }
}