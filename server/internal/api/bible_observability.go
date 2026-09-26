package api

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
)

type bibleOperationMetric struct { Requests int64 `json:"requests"`; ClientErrors int64 `json:"client_errors"`; ServerErrors int64 `json:"server_errors"`; LatencyTotalMS int64 `json:"latency_total_ms"`; LatencyMaxMS int64 `json:"latency_max_ms"` }
type bibleOperationMetrics struct { mu sync.Mutex; values map[string]bibleOperationMetric }
func newBibleOperationMetrics()*bibleOperationMetrics{return &bibleOperationMetrics{values:map[string]bibleOperationMetric{}}}
func (m *bibleOperationMetrics)add(key string,status int,d time.Duration){if m==nil{return};m.mu.Lock();v:=m.values[key];v.Requests++;if status>=400&&status<500{v.ClientErrors++};if status>=500{v.ServerErrors++};ms:=d.Milliseconds();v.LatencyTotalMS+=ms;if ms>v.LatencyMaxMS{v.LatencyMaxMS=ms};m.values[key]=v;m.mu.Unlock()}
func (m *bibleOperationMetrics)snapshot()map[string]bibleOperationMetric{out:=map[string]bibleOperationMetric{};if m==nil{return out};m.mu.Lock();for k,v:=range m.values{out[k]=v};m.mu.Unlock();return out}

type bibleStatusWriter struct{http.ResponseWriter;status int}
func (w *bibleStatusWriter)WriteHeader(code int){if w.status!=0{return};w.status=code;w.ResponseWriter.WriteHeader(code)}
func (w *bibleStatusWriter)Write(b []byte)(int,error){if w.status==0{w.WriteHeader(http.StatusOK)};return w.ResponseWriter.Write(b)}

func (h *Handler) instrumentBibleRoute(method,path string,next http.Handler)http.Handler{	key:=strings.ToUpper(method)+" "+path
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){start:=time.Now();rec:=&bibleStatusWriter{ResponseWriter:w};next.ServeHTTP(rec,r);status:=rec.status;if status==0{status=http.StatusOK};h.bibleMetrics.add(key,status,time.Since(start))})
}

func (h *Handler) adminBibleMetrics(w http.ResponseWriter,r *http.Request){	values:=h.bibleMetrics.snapshot();keys:=make([]string,0,len(values));for key:=range values{keys=append(keys,key)};sort.Strings(keys);items:=make([]map[string]any,0,len(keys));for _,key:=range keys{items=append(items,map[string]any{"operation":key,"metrics":values[key]})};httpx.WriteJSON(w,http.StatusOK,map[string]any{"window":"process_lifetime","operations":items,"private_payloads_recorded":false,"at":time.Now().UTC()})}
