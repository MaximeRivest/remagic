// Package webconfig serves a one-shot local web form for an app's settings:
// type the API key on a real keyboard (laptop or phone via QR) and the CLI
// writes the result straight to the tablet over SSH. Nothing is hosted; the
// key never leaves your machines.
package webconfig

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/maximerivest/remagic/cli/internal/device"
)

// A Field of the settings form. Kind: "text", "secret", or "select".
type Field struct {
	Key         string
	Label       string
	Kind        string
	Placeholder string
	Help        string
	Options     []string
}

// Schema describes one configurable app.
type Schema struct {
	App        string
	RemotePath string
	Fields     []Field
	// Presets fill several fields at once from one dropdown.
	Presets []Preset
}

type Preset struct {
	Name   string
	Values map[string]string
}

// Riddle is the only schema for now; new apps add one entry here (and later,
// a schema block in the catalog).
func Riddle() Schema {
	return Schema{
		App:        "riddle",
		RemotePath: device.ApploadDir + "/riddle/oracle.env",
		Fields: []Field{
			{Key: "RIDDLE_OPENAI_KEY", Label: "API key", Kind: "secret",
				Placeholder: "sk-… / AIza…", Help: "Stored only on the tablet."},
			{Key: "RIDDLE_OPENAI_BASE", Label: "API base URL", Kind: "text",
				Placeholder: "https://api.openai.com/v1"},
			{Key: "RIDDLE_OPENAI_MODEL", Label: "Model", Kind: "text",
				Placeholder: "gpt-4o-mini", Help: "Must be vision-capable."},
			{Key: "RIDDLE_OPENAI_REASONING", Label: "Reasoning effort", Kind: "select",
				Options: []string{"", "low", "medium", "high"},
				Help:    "Only for thinking models (Gemini 3.x, o-series). Leave empty otherwise."},
			{Key: "RIDDLE_OPENAI_MAX_TOKENS", Label: "Max tokens", Kind: "text",
				Placeholder: "2000", Help: "Runaway guard. Thinking models count hidden reasoning tokens against it — keep it roomy."},
		},
		Presets: []Preset{
			{"Gemini", map[string]string{
				"RIDDLE_OPENAI_BASE":      "https://generativelanguage.googleapis.com/v1beta/openai",
				"RIDDLE_OPENAI_MODEL":     "gemini-3.5-flash",
				"RIDDLE_OPENAI_REASONING": "low",
			}},
			{"OpenAI", map[string]string{
				"RIDDLE_OPENAI_BASE":      "https://api.openai.com/v1",
				"RIDDLE_OPENAI_MODEL":     "gpt-4o-mini",
				"RIDDLE_OPENAI_REASONING": "",
			}},
			{"OpenRouter", map[string]string{
				"RIDDLE_OPENAI_BASE":      "https://openrouter.ai/api/v1",
				"RIDDLE_OPENAI_MODEL":     "openai/gpt-4o-mini",
				"RIDDLE_OPENAI_REASONING": "",
			}},
		},
	}
}

// Serve runs the form until one successful save (or timeout), then returns.
// Returns the URL it served on via the urlReady callback before blocking.
func Serve(dev *device.Device, s Schema, lan bool, urlReady func(url string)) error {
	current, _ := dev.Run("cat " + s.RemotePath + " 2>/dev/null")
	vals := ParseEnv(current)

	// Random path segment: on a LAN bind, a port-scanner shouldn't stumble
	// straight onto a form that writes to the tablet.
	tok := make([]byte, 8)
	rand.Read(tok)
	base := "/s/" + hex.EncodeToString(tok)

	bind := "127.0.0.1:0"
	if lan {
		bind = "0.0.0.0:0"
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	host := "127.0.0.1"
	if lan {
		if ip := OutboundIP(); ip != "" {
			host = ip
		}
	}
	url := fmt.Sprintf("http://%s:%d%s", host, port, base)

	done := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		renderForm(w, s, vals, base, "")
	})
	mux.HandleFunc(base+"/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, base, http.StatusSeeOther)
			return
		}
		r.ParseForm()
		for _, f := range s.Fields {
			vals[f.Key] = strings.TrimSpace(r.Form.Get(f.Key))
		}
		content := BuildEnv(s, vals, current)
		if err := dev.Push([]byte(content), s.RemotePath, "600"); err != nil {
			renderForm(w, s, vals, base, "write to tablet failed: "+err.Error())
			return
		}
		fmt.Fprint(w, savedHTML)
		go func() {
			time.Sleep(500 * time.Millisecond)
			done <- nil
		}()
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	urlReady(url)

	var serveErr error
	select {
	case serveErr = <-done:
	case <-time.After(15 * time.Minute):
		serveErr = fmt.Errorf("timed out after 15 minutes with no save")
	}
	srv.Close()
	return serveErr
}

// OutboundIP finds this machine's LAN-facing IPv4 without sending anything.
func OutboundIP() string {
	conn, err := net.Dial("udp", "203.0.113.1:9")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// ParseEnv reads KEY=VALUE lines, ignoring comments and blanks.
func ParseEnv(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// BuildEnv renders the schema's fields (skipping empties), then preserves any
// unrelated keys the existing file carried.
func BuildEnv(s Schema, vals map[string]string, previous string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s configuration — written by remagic on %s\n",
		s.App, time.Now().Format("2006-01-02 15:04"))
	known := map[string]bool{}
	for _, f := range s.Fields {
		known[f.Key] = true
		if v := vals[f.Key]; v != "" {
			fmt.Fprintf(&b, "%s=%s\n", f.Key, v)
		}
	}
	extras := ParseEnv(previous)
	var keys []string
	for k := range extras {
		if !known[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, extras[k])
	}
	return b.String()
}

func renderForm(w http.ResponseWriter, s Schema, vals map[string]string, base, errMsg string) {
	data := struct {
		Schema  Schema
		Vals    map[string]string
		Base    string
		Err     string
		Presets template.JS
	}{s, vals, base, errMsg, template.JS(presetsJSON(s))}
	formTmpl.Execute(w, data)
}

func presetsJSON(s Schema) string {
	var b strings.Builder
	b.WriteString("{")
	for i, p := range s.Presets {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%q:{", p.Name)
		j := 0
		for k, v := range p.Values {
			if j > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "%q:%q", k, v)
			j++
		}
		b.WriteString("}")
	}
	b.WriteString("}")
	return b.String()
}

var formTmpl = template.Must(template.New("form").Parse(`<!doctype html>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>remagic — {{.Schema.App}}</title>
<style>
  body{font:16px/1.5 system-ui,sans-serif;max-width:26rem;margin:2rem auto;padding:0 1rem;color:#222}
  h1{font-size:1.3rem} label{display:block;margin:1rem 0 .2rem;font-weight:600}
  input,select{width:100%;padding:.55rem;font-size:1rem;border:1px solid #bbb;border-radius:6px;box-sizing:border-box}
  .help{font-size:.8rem;color:#666;margin-top:.15rem}
  button{margin-top:1.4rem;width:100%;padding:.7rem;font-size:1.05rem;border:0;border-radius:8px;background:#222;color:#fff}
  .err{background:#fee;border:1px solid #c00;padding:.6rem;border-radius:6px;margin-top:1rem}
  .row{display:flex;gap:.5rem;align-items:center}
</style>
<h1>The Diary — oracle settings</h1>
{{if .Err}}<div class="err">{{.Err}}</div>{{end}}
<label>Provider preset</label>
<select id="preset" onchange="applyPreset()">
  <option value="">— pick one, or fill fields yourself —</option>
  {{range .Schema.Presets}}<option>{{.Name}}</option>{{end}}
</select>
<form method="post" action="{{.Base}}/save">
{{range .Schema.Fields}}
  <label>{{.Label}}</label>
  {{if eq .Kind "select"}}
    <select name="{{.Key}}" id="{{.Key}}">
      {{$cur := index $.Vals .Key}}
      {{range .Options}}<option value="{{.}}" {{if eq . $cur}}selected{{end}}>{{if eq . ""}}(none){{else}}{{.}}{{end}}</option>{{end}}
    </select>
  {{else if eq .Kind "secret"}}
    <div class="row">
      <input name="{{.Key}}" id="{{.Key}}" type="password" value="{{index $.Vals .Key}}" placeholder="{{.Placeholder}}" autocomplete="off">
      <button type="button" style="width:auto;margin:0;padding:.5rem .7rem" onclick="var i=document.getElementById('{{.Key}}');i.type=i.type=='password'?'text':'password'">👁</button>
    </div>
  {{else}}
    <input name="{{.Key}}" id="{{.Key}}" value="{{index $.Vals .Key}}" placeholder="{{.Placeholder}}">
  {{end}}
  {{if .Help}}<div class="help">{{.Help}}</div>{{end}}
{{end}}
  <button>Save to tablet</button>
</form>
<script>
const PRESETS = {{.Presets}};
function applyPreset(){
  const p = PRESETS[document.getElementById('preset').value];
  if(!p) return;
  for(const [k,v] of Object.entries(p)){
    const el = document.getElementById(k);
    if(el) el.value = v;
  }
}
</script>
`))

const savedHTML = `<!doctype html>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>body{font:18px/1.6 system-ui,sans-serif;max-width:26rem;margin:3rem auto;padding:0 1rem;text-align:center}</style>
<h1>✓ Saved to the tablet</h1>
<p>Relaunch the app to pick up the new settings.<br>You can close this page.</p>`
