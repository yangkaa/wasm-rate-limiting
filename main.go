package main

import (
	"encoding/json"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"os"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

type vmContext struct {
	// Embed the default VM context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultVMContext
}

// Override types.DefaultVMContext.
func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

type pluginContext struct {
	// Embed the default plugin context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultPluginContext
	// the remaining token for rate limiting, refreshed periodically.
	remainToken int
	// // the preconfigured request per second for rate limiting.
	// requestPerSecond int
	// NOTE(jianfeih): any concerns about the threading and mutex usage for tinygo wasm?
	// the last time the token is refilled with `requestPerSecond`.
	lastRefillNanoSec int64
	rels              map[string]string
}

// Override types.DefaultPluginContext.
func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpHeaders{contextID: contextID, pluginContext: p}
}

type httpHeaders struct {
	// Embed the default http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext
	contextID     uint32
	pluginContext *pluginContext
}

// Additional headers supposed to be injected to response headers.
var additionalHeaders = map[string]string{
	"who-am-i":    "wasm-extension",
	"injected-by": "istio-api!",
}

func (ctx *httpHeaders) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
	for key, value := range additionalHeaders {
		proxywasm.AddHttpResponseHeader(key, value)
	}
	return types.ActionContinue
}

//var relations sync.Map

func (ctx *httpHeaders) OnHttpRequestHeaders(int, bool) types.Action {
	traceType := os.Getenv("TraceType")
	if traceType == "" {
		traceType = "X-B3-Traceid"
	}
	grayHeaders := new([][]map[string]string)
	grayHeaderJson := os.Getenv("GrayHeader")
	err := json.Unmarshal([]byte(grayHeaderJson), grayHeaders)
	if err != nil {
		proxywasm.LogErrorf("json unmarshal failure: ", err)
	}
	xreq_id, err := proxywasm.GetHttpRequestHeader(traceType)
	if err != nil || xreq_id == "" {
		proxywasm.LogErrorf("Get X-B3-Traceid err: [%v], xreq_id [%v]", err, xreq_id)
		return types.ActionContinue
	}
	data, cas, err := proxywasm.GetSharedData(xreq_id)
	if err != nil {
		proxywasm.LogErrorf("proxywasm.GetSharedData(xreq_id) err [%v]", err)
	} else {
		proxywasm.LogErrorf("proxywasm.GetSharedData have xreq_id(%v) data is [%v]", xreq_id, data)
		proxywasm.AddHttpRequestHeader("Gray", "true")
		return types.ActionContinue
	}
	grayTraffic := false
	for _, grayHeader := range *grayHeaders {
		match := true
		for _, headers := range grayHeader {
			headerValue, err := proxywasm.GetHttpRequestHeader(headers["header_key"])
			if err != nil || headerValue == "" {
				proxywasm.LogErrorf("get http request header failure %v", err)
				match = false
				break
			}
		}
		if match {
			grayTraffic = true
			break
		}
	}
	gray, err := proxywasm.GetHttpRequestHeader("Gray")
	if err != nil || gray == "" {
		proxywasm.LogErrorf("Get X-Forwarded-Host err 5: [%v], host [%v]", err, gray)
	}
	if grayTraffic || gray != "" {
		err := proxywasm.SetSharedData(xreq_id, []byte("true"), cas)
		if err != nil {
			proxywasm.LogErrorf("proxywasm.SetSharedData error [%v]", err)
		}
		proxywasm.LogErrorf("proxywasm.SetSharedData xreq_id [%v]", xreq_id)
	}
	return types.ActionContinue
}
