package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ParticleMedia/tikv-proxy/common"
	"github.com/pingcap/tidb/config"
	"github.com/pingcap/tidb/store/tikv"
	"github.com/rcrowley/go-metrics"
	"math/rand"
	"net/http"
	"strings"
	"time"
	"github.com/golang/glog"
)

type ProxyServer struct {
	cli *tikv.RawKVClient;
    mux *http.ServeMux
}

type ServerResult struct {
	Status int `json:"status"`
	Message string `json:"msg,omitempty"`
	Data map[string]interface{} `json:"data,omitempty"`
}

type KVPair struct {
	Key string `json:"key"`
	Value string `json:"value"`
}

type LogInfo map[string]interface{}

type HandleFuncInner func(http.ResponseWriter, *http.Request, *LogInfo) int

func NewLogInfo() *LogInfo  {
	return &LogInfo{}
}

func (l *LogInfo) set(key string, value interface{}) {
	(*l)[key] = value
}

func (l *LogInfo) toString() string {
	if l == nil || len(*l) == 0 {
		return ""
	}
	splits := make([]string, 0, len(*l))
	for k, v := range(*l) {
		splits = append(splits, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(splits, " ")
}

func BuildTikvClient() (*tikv.RawKVClient, error) {
	// create tikv client
	timeout := time.Duration(common.ProxyConfig.Tikv.ConnTimeout)  * time.Millisecond
	ch := make(chan interface{})
	go func(pdAddr []string) {
		cli, err := tikv.NewRawKVClient(pdAddr, config.Security{});
		if err != nil {
			ch <- err
		} else {
			ch <- cli
		}
	}(common.ProxyConfig.Tikv.PdAddrs)

	select {
	case ret := <-ch:
		if err, ok := ret.(error); ok {
			return nil, err
		} else if cli, ok := ret.(*tikv.RawKVClient); ok {
			return cli, nil
		} else {
			return nil, errors.New("Unknow error, should not happen")
		}
	case <-time.After(timeout):
		return nil, errors.New("create tikv client timeout")
	}
	return nil, errors.New("Unknow error, should not happen")
}

func NewProxyServer(mux *http.ServeMux) (*ProxyServer, error) {
	cli, err := BuildTikvClient()
	if err != nil {
		return nil, err
	}
	server := &ProxyServer{
		cli: cli,
		mux: mux,
	}
	err = server.Register()
	return server, err
}

func (s *ProxyServer) Close() error {
	if s != nil && s.cli != nil {
		err := s.cli.Close()
		s.cli = nil
		return err
	}
	return nil
}

func (s *ProxyServer) Register() error {
	s.mux.HandleFunc("/ping", s.wrapper(s.ping, "ping"))
	s.mux.HandleFunc("/get", s.wrapper(s.get, "get"))
	s.mux.HandleFunc("/del", s.wrapper(s.del, "del"))
	s.mux.HandleFunc("/set", s.wrapper(s.set, "set"))
	return nil
}

func (s *ProxyServer) wrapper(handlerFunc HandleFuncInner, metricName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics.GetOrRegisterMeter(fmt.Sprintf("%s.qps", metricName), nil).Mark(1)
		l := NewLogInfo()
		start := time.Now()
		code := handlerFunc(w, r, l)
		duration := time.Since(start)
		cost := duration.Nanoseconds() / 1000
		metrics.GetOrRegisterTimer(fmt.Sprintf("%s.latency", metricName), nil).Update(duration)

		if code >= 400 && code <= 499 {
			// 请求不合法
			metrics.GetOrRegisterMeter(fmt.Sprintf("%s.invalid.qps", metricName), nil).Mark(1)
		} else if code >= 500 && code <= 599 {
			// 服务端错误
			metrics.GetOrRegisterMeter(fmt.Sprintf("%s.error.qps", metricName), nil).Mark(1)
		}

		randInt := uint32(rand.Intn(100))
		if randInt <= common.ProxyConfig.Log.SampleRate {
			// 打印日志抽样控制
			l.set("accept_encoding", r.Header.Get("Accept-Encoding"))
			remote := r.Header.Get("x-forwarded-for")
			glog.Infof("method=%s uri=%s remote=%s from=%s status=%d cost=%d %s", r.Method, r.RequestURI, remote, r.RemoteAddr, code, cost, l.toString())
		}
	}
}

func (s *ProxyServer) writeResponse(w http.ResponseWriter, statusCode int, result *ServerResult) int {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if result != nil {
		encoder := json.NewEncoder(w)
		err := encoder.Encode(*result)
		if err != nil {
			glog.Warningf("encode response %+v to json error: %+v", result, err)
			return 500
		}
		return statusCode
	}
	return statusCode
}

func (s *ProxyServer) responseOK(w http.ResponseWriter) int {
	return s.writeResponse(w, 200, &ServerResult{
		Status: 0,
		Message: "OK",
		Data: nil,
	})
}

func (s *ProxyServer) responseError(w http.ResponseWriter, statusCode int, message string, l *LogInfo) int {
	l.set("message", message)
	return s.writeResponse(w, statusCode, &ServerResult{
		Status: -1,
		Message: message,
		Data: nil,
	})
}

func (s *ProxyServer) ping(w http.ResponseWriter, r *http.Request, l *LogInfo) int {
	return s.writeResponse(w, 200, &ServerResult{
		Status: 0,
		Message: "pong",
		Data: nil,
	})
}

func (s *ProxyServer) get(w http.ResponseWriter, r *http.Request, l *LogInfo) int {
	if r.Method != "GET" {
		return s.responseError(w, 405, "Only GET is allowed", l)
	}
	r.ParseForm()
	keys := strings.Split(r.Form.Get("keys"), ",")
	l.set("keys", len(keys))
	metrics.GetOrRegisterMeter("get.kps", nil).Mark(int64(len(keys)))
	if len(keys) == 0 {
		return s.responseError(w, 400, "no keys", l)
	}
	if common.ProxyConfig.Limit.MaxGetKeys > 0 && int32(len(keys)) > common.ProxyConfig.Limit.MaxGetKeys {
		return s.responseError(w, 400, "key count exceed limit", l)
	}

	format := r.Form.Get("format")
	l.set("format", format)
	if len(format) == 0 {
		format = "string"
	}

	var valueParseFunc func([]byte) string
	switch strings.ToLower(format) {
	case "string":
		valueParseFunc = func (v []byte) string { return string(v) }
	case "float_arr":
		valueParseFunc = func (v []byte) string { return string(v) }
	case "base64":
		valueParseFunc = base64.StdEncoding.EncodeToString
	default:
		return s.responseError(w, 400, fmt.Sprintf("unsupported format: %s", format), l)
	}

	tikvKeys := make([][]byte, 0, len(keys))
	for _, k := range(keys) {
		trimed := strings.TrimSpace(k)
		if len(trimed) == 0 {
			continue
		}
		tikvKeys = append(tikvKeys, []byte(trimed))
	}

	values, err := s.cli.BatchGet(tikvKeys)
	if err != nil {
		glog.Warningf("BatchGet tikv error: %+v", err)
		return s.responseError(w, 500, err.Error(), l)
	}

	var valueSize int = 0
	for i, value := range(values) {
		valueSize += len(tikvKeys[i])
		valueSize += len(value)
	}
	l.set("size", valueSize)
	metrics.GetOrRegisterMeter("get.bps", nil).Mark(int64(valueSize))

	result := make(map[string]interface{})

	for i, value := range(values) {
		key := string(tikvKeys[i])
		result[key] = valueParseFunc(value)
	}

	return s.writeResponse(w, 200, &ServerResult{
		Status: 0,
		Message: "",
		Data: result,
	})
}

func (s *ProxyServer) del(w http.ResponseWriter, r *http.Request, l *LogInfo) int {
	if r.Method != "DELETE" {
		return s.responseError(w, 405, "Only DELETE is allowed", l)
	}

	r.ParseForm()
	keys := strings.Split(r.Form.Get("keys"), ",")
	l.set("keys", len(keys))
	metrics.GetOrRegisterMeter("del.kps", nil).Mark(int64(len(keys)))
	if len(keys) == 0 {
		return s.responseError(w, 400, "no keys", l)
	}
	if common.ProxyConfig.Limit.MaxDelKeys > 0 && int32(len(keys)) > common.ProxyConfig.Limit.MaxDelKeys {
		return s.responseError(w, 400, "key count exceed limit", l)
	}

	tikvKeys := make([][]byte, 0, len(keys))
	for _, k := range(keys) {
		trimed := strings.TrimSpace(k)
		if len(trimed) == 0 {
			continue
		}
		tikvKeys =  append(tikvKeys, []byte(trimed))
	}

	err := s.cli.BatchDelete(tikvKeys)
	if err != nil {
		glog.Warningf("BatchDelete tikv error: %+v", err)
		return s.responseError(w, 500, err.Error(), l)
	}
	return s.responseOK(w)
}

func parseValue(value string, format string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "string":
		return []byte(value), nil
	case "base64":
		return base64.StdEncoding.DecodeString(value)
	default:
		return nil, errors.New(fmt.Sprintf("unsupported format: %s", format))
	}
}

func (s *ProxyServer) set(w http.ResponseWriter, r *http.Request, l *LogInfo) int {
	if r.Method != "POST" {
		return s.responseError(w, 405, "Only POST is allowed", l)
	}
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "application/json") {
		return s.responseError(w, 405, "Content-Type should be json", l)
	}

	r.ParseForm()
	format := r.Form.Get("format")
	l.set("format", format)
	if len(format) == 0 {
		format = "string"
	}

	var data []KVPair
	decoder := json.NewDecoder(r.Body)
    err := decoder.Decode(&data)
	l.set("keys", len(data))
	metrics.GetOrRegisterMeter("set.kps", nil).Mark(int64(len(data)))

	if common.ProxyConfig.Limit.MaxSetKeys > 0 && int32(len(data)) > common.ProxyConfig.Limit.MaxSetKeys {
		return s.responseError(w, 400, "key count exceed limit", l)
	}

	tikvKeys := make([][]byte, 0, len(data))
	tikvVals := make([][]byte, 0, len(data))
	var valueSize int = 0
	for _, kv := range(data) {
		if len(kv.Key) == 0 || len(kv.Value) == 0 {
			return s.responseError(w, 400, "invalid key or value", l)
		}
		value, valErr := parseValue(kv.Value, format)
		if valErr != nil {
			return s.responseError(w, 400, valErr.Error(), l)
		}

		key := []byte(kv.Key)
		tikvKeys = append(tikvKeys, key)
		tikvVals = append(tikvVals, value)
		valueSize += (len(key) + len(value))
	}
	l.set("size", valueSize)
	metrics.GetOrRegisterMeter("set.bps", nil).Mark(int64(valueSize))

	err = s.cli.BatchPut(tikvKeys, tikvVals)
	if err != nil {
		glog.Warningf("BatchPut tikv error: %+v", err)
		return s.responseError(w, 500, err.Error(), l)
	}
	return s.responseOK(w)
}