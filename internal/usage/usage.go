package usage

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/khashino/AISH/internal/provider"
	"github.com/khashino/AISH/internal/securestore"
)

type Record struct {
	Time         time.Time `json:"time"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Operation    string    `json:"operation"`
	TaskID       string    `json:"task_id,omitempty"`
	Session      string    `json:"session,omitempty"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	Estimated    bool      `json:"estimated"`
	DurationMS   int64     `json:"duration_ms"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
}

type Summary struct {
	Requests, InputTokens, OutputTokens, TotalTokens int
	DurationMS int64
	CostUSD float64
	EstimatedRecords int
}

func path() (string, error) {
	d, err := os.UserCacheDir(); if err != nil { return "", err }
	return filepath.Join(d, "aish", "usage.json"), nil
}
func ReadAll() ([]Record, error) {
	p, err := path(); if err != nil { return nil, err }
	b, err := os.ReadFile(p); if errors.Is(err, os.ErrNotExist) { return nil, nil }; if err != nil { return nil, err }
	b, err = securestore.Decrypt(b); if err != nil { return nil, err }
	var out []Record; if len(b)==0 { return out,nil }; err=json.Unmarshal(b,&out); return out,err
}
func Save(records []Record) error {
	p, err := path(); if err != nil { return err }
	if err=os.MkdirAll(filepath.Dir(p),0700); err!=nil{return err}
	b,err:=json.MarshalIndent(records,"","  "); if err!=nil{return err}; b,err=securestore.Encrypt(b); if err!=nil{return err}
	return os.WriteFile(p,b,0600)
}
func Append(r Record) error { xs,err:=ReadAll(); if err!=nil{return err}; xs=append(xs,r); return Save(xs) }
func Reset() error { return Save(nil) }
func Estimate(messages []provider.Message, answer string) provider.Usage {
	in:=0; for _,m:=range messages { in += approximate(m.Content) }
	out:=approximate(answer); return provider.Usage{InputTokens:in,OutputTokens:out,TotalTokens:in+out,Estimated:true}
}
func approximate(s string) int { if strings.TrimSpace(s)=="" {return 0}; n:=(len([]rune(s))+3)/4; if n<1{return 1}; return n }
func Summarize(xs []Record) Summary { var s Summary; for _,r:=range xs{s.Requests++;s.InputTokens+=r.InputTokens;s.OutputTokens+=r.OutputTokens;s.TotalTokens+=r.TotalTokens;s.DurationMS+=r.DurationMS;s.CostUSD+=r.CostUSD;if r.Estimated{s.EstimatedRecords++}}; return s }
func Today(xs []Record, now time.Time) []Record { y,m,d:=now.Date(); loc:=now.Location(); start:=time.Date(y,m,d,0,0,0,0,loc); var out []Record; for _,r:=range xs {if !r.Time.Before(start){out=append(out,r)}}; return out }
func Filter(xs []Record, taskID, session string) []Record { var out []Record; for _,r:=range xs{if taskID!=""&&r.TaskID!=taskID{continue};if session!=""&&r.Session!=session{continue};out=append(out,r)};return out }
func ByProvider(xs []Record) map[string]Summary { out:=map[string]Summary{}; for _,r:=range xs{s:=out[r.Provider];s.Requests++;s.InputTokens+=r.InputTokens;s.OutputTokens+=r.OutputTokens;s.TotalTokens+=r.TotalTokens;s.DurationMS+=r.DurationMS;s.CostUSD+=r.CostUSD;if r.Estimated{s.EstimatedRecords++};out[r.Provider]=s};return out }
func Export(format, dest string, xs []Record) error {
	if dest=="" { dest="aish-usage."+format }
	f,err:=os.Create(dest); if err!=nil{return err}; defer f.Close()
	switch format {
	case "json": enc:=json.NewEncoder(f);enc.SetIndent("","  ");return enc.Encode(xs)
	case "csv": w:=csv.NewWriter(f); defer w.Flush(); _=w.Write([]string{"time","provider","model","operation","task_id","session","input_tokens","output_tokens","total_tokens","estimated","duration_ms","cost_usd"}); for _,r:=range xs{_ = w.Write([]string{r.Time.Format(time.RFC3339),r.Provider,r.Model,r.Operation,r.TaskID,r.Session,strconv.Itoa(r.InputTokens),strconv.Itoa(r.OutputTokens),strconv.Itoa(r.TotalTokens),strconv.FormatBool(r.Estimated),strconv.FormatInt(r.DurationMS,10),fmt.Sprintf("%.8f",r.CostUSD)})}; return w.Error()
	default:return fmt.Errorf("unsupported export format %q",format)
	}
}
func SortedProviders(m map[string]Summary) []string { ks:=make([]string,0,len(m));for k:=range m{ks=append(ks,k)};sort.Strings(ks);return ks }
