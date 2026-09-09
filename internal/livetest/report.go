package livetest

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thomas-huang/caosi/internal/config"
)

const (
	outcomePass = "PASS"
	outcomeFail = "FAIL"
	outcomeSkip = "SKIP"
)

type event struct {
	Provider string
	Upstream config.Protocol
	Client   config.Protocol
	Stream   bool
	Cell     string // empty = bundle
	Outcome  string
	Err      string
}

type recorder struct {
	mu           sync.Mutex
	Started      time.Time
	Dir          string
	Missing      []config.Protocol
	Placeholders []string
	Events       []event
}

func newRecorder(dir string, file *config.File) *recorder {
	return &recorder{
		Started:      time.Now(),
		Dir:          dir,
		Missing:      missingUpstream(file),
		Placeholders: placeholderNames(file),
	}
}

func (r *recorder) add(e event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Events = append(r.Events, e)
}

func (r *recorder) markdown() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	pass, fail, skip := 0, 0, 0
	for _, e := range r.Events {
		if e.Cell != "" {
			continue
		}
		switch e.Outcome {
		case outcomePass:
			pass++
		case outcomeFail:
			fail++
		case outcomeSkip:
			skip++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# caosi live 报告\n\n")
	fmt.Fprintf(&b, "- 时间: %s\n", r.Started.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "- 快照: %s\n", r.Dir)
	fmt.Fprintf(&b, "- 结果: **%d 通过** · **%d 失败** · %d 跳过（bundle 枪）\n", pass, fail, skip)
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "- 未配置的上游协议（本趟不测，**不是失败**）: %s\n", joinProtocols(r.Missing))
	}
	if len(r.Placeholders) > 0 {
		fmt.Fprintf(&b, "- 占位密钥已跳过: %s\n", strings.Join(r.Placeholders, ", "))
	}
	b.WriteString("\n## 矩阵\n\n")
	b.WriteString("| Provider | 上游 | 客户端 | JSON | Stream |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, row := range matrixRows(r.Events) {
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s | %s |\n",
			row.provider, row.upstream, row.client, badge(row.json), badge(row.stream))
	}
	fails := failEvents(r.Events)
	if len(fails) > 0 {
		b.WriteString("\n## 失败明细\n")
		cur := ""
		for _, e := range fails {
			head := e.Provider + " / " + string(e.Client) + " / " + modeName(e.Stream)
			if e.Cell != "" {
				head += " / " + e.Cell
			} else {
				head += " / bundle"
			}
			if head != cur {
				fmt.Fprintf(&b, "\n### %s\n\n", head)
				cur = head
			}
			fmt.Fprintf(&b, "```\n%s\n```\n", strings.TrimSpace(e.Err))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (r *recorder) html() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	pass, fail, skip := 0, 0, 0
	for _, e := range r.Events {
		if e.Cell != "" {
			continue
		}
		switch e.Outcome {
		case outcomePass:
			pass++
		case outcomeFail:
			fail++
		case outcomeSkip:
			skip++
		}
	}
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8"><title>caosi live</title>
<style>
body{font-family:ui-sans-serif,system-ui,sans-serif;margin:24px;color:#111}
h1{font-size:1.4rem}
.meta{color:#444;line-height:1.6}
table{border-collapse:collapse;margin-top:12px}
th,td{border:1px solid #ddd;padding:6px 10px;text-align:left}
th{background:#f4f4f4}
.PASS{background:#d4edda;color:#155724;font-weight:600}
.FAIL{background:#f8d7da;color:#721c24;font-weight:600}
.SKIP{background:#e9ecef;color:#555}
pre{background:#f8f8f8;padding:10px;overflow:auto;white-space:pre-wrap}
.note{background:#fff3cd;padding:8px 12px;margin:12px 0}
</style></head><body>`)
	fmt.Fprintf(&b, "<h1>caosi live 报告</h1><div class=\"meta\">")
	fmt.Fprintf(&b, "<div>时间: %s</div>", html.EscapeString(r.Started.Format("2006-01-02 15:04:05")))
	fmt.Fprintf(&b, "<div>快照: %s</div>", html.EscapeString(r.Dir))
	fmt.Fprintf(&b, "<div>结果: <b>%d 通过</b> · <b>%d 失败</b> · %d 跳过</div></div>", pass, fail, skip)
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "<div class=\"note\">未配置的上游协议（本趟不测，不是失败）: %s</div>", html.EscapeString(joinProtocols(r.Missing)))
	}
	if len(r.Placeholders) > 0 {
		fmt.Fprintf(&b, "<div class=\"note\">占位密钥已跳过: %s</div>", html.EscapeString(strings.Join(r.Placeholders, ", ")))
	}
	b.WriteString("<h2>矩阵</h2><table><tr><th>Provider</th><th>上游</th><th>客户端</th><th>JSON</th><th>Stream</th></tr>")
	for _, row := range matrixRows(r.Events) {
		fmt.Fprintf(&b, "<tr><td>%s</td><td><code>%s</code></td><td><code>%s</code></td><td class=\"%s\">%s</td><td class=\"%s\">%s</td></tr>",
			html.EscapeString(row.provider), html.EscapeString(string(row.upstream)), html.EscapeString(string(row.client)),
			cssOutcome(row.json), badge(row.json), cssOutcome(row.stream), badge(row.stream))
	}
	b.WriteString("</table>")
	fails := failEvents(r.Events)
	if len(fails) > 0 {
		b.WriteString("<h2>失败明细</h2>")
		for _, e := range fails {
			head := e.Provider + " / " + string(e.Client) + " / " + modeName(e.Stream)
			if e.Cell != "" {
				head += " / " + e.Cell
			} else {
				head += " / bundle"
			}
			fmt.Fprintf(&b, "<h3>%s</h3><pre>%s</pre>", html.EscapeString(head), html.EscapeString(strings.TrimSpace(e.Err)))
		}
	}
	b.WriteString("</body></html>")
	return b.String()
}

func (r *recorder) write() (mdPath, htmlPath string, err error) {
	mdPath = filepath.Join(r.Dir, reportMDName)
	htmlPath = filepath.Join(r.Dir, reportHTMLName)
	if err = os.WriteFile(mdPath, []byte(r.markdown()), 0o644); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(htmlPath, []byte(r.html()), 0o644); err != nil {
		return mdPath, "", err
	}
	return mdPath, htmlPath, nil
}

type matrixRow struct {
	provider, client string
	upstream         config.Protocol
	json, stream     string
}

func matrixRows(events []event) []matrixRow {
	type key struct {
		p, c string
		u    config.Protocol
	}
	order := []key{}
	seen := map[key]*matrixRow{}
	for _, e := range events {
		if e.Cell != "" {
			continue
		}
		k := key{e.Provider, string(e.Client), e.Upstream}
		row, ok := seen[k]
		if !ok {
			row = &matrixRow{provider: e.Provider, client: string(e.Client), upstream: e.Upstream, json: "—", stream: "—"}
			seen[k] = row
			order = append(order, k)
		}
		if e.Stream {
			row.stream = e.Outcome
		} else {
			row.json = e.Outcome
		}
	}
	out := make([]matrixRow, 0, len(order))
	for _, k := range order {
		out = append(out, *seen[k])
	}
	return out
}

func failEvents(events []event) []event {
	var out []event
	for _, e := range events {
		if e.Outcome == outcomeFail {
			out = append(out, e)
		}
	}
	return out
}

func badge(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func cssOutcome(s string) string {
	switch s {
	case outcomePass, outcomeFail, outcomeSkip:
		return s
	}
	return ""
}

func modeName(stream bool) string {
	if stream {
		return "stream"
	}
	return "json"
}

func joinProtocols(ps []config.Protocol) string {
	var s []string
	for _, p := range ps {
		s = append(s, string(p))
	}
	return strings.Join(s, ", ")
}
