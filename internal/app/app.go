package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"caosi/internal/config"
	"caosi/internal/server"
)

const defaultPort = 9999
const defaultListen = "127.0.0.1"

// Main is the testable CLI entry. args[0] is the program name.
func Main(args []string, stdout, stderr io.Writer) int {
	return MainContext(context.Background(), args, stdout, stderr)
}

func MainContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	name := "caosi"
	if len(args) > 0 {
		name = args[0]
	}
	rest := []string{}
	if len(args) > 1 {
		rest = args[1:]
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { writeHelp(stderr) }

	configDir := fs.String("config-dir", "", "配置目录（默认: ~/.caosi）")
	port := fs.Int("port", defaultPort, "监听端口（默认: 9999）")
	listen := fs.String("listen", defaultListen, "只允许 loopback：127.0.0.1 或 ::1")
	logLevel := fs.String("log-level", "info", "debug|info|warn|error")

	if err := fs.Parse(rest); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "caosi: 不认识多余参数 %q\n\n", strings.Join(fs.Args(), " "))
		writeHelp(stderr)
		return 2
	}

	if !isLoopback(*listen) {
		fmt.Fprintf(stderr, "caosi: --listen 只能是 127.0.0.1 或 ::1，拒绝绑到 %s（会把密钥暴露到局域网）。\n", *listen)
		return 2
	}

	dir := strings.TrimSpace(*configDir)
	if dir == "" {
		d, err := config.DefaultConfigDir()
		if err != nil {
			fmt.Fprintf(stderr, "caosi: 找不到用户目录: %v\n", err)
			return 1
		}
		dir = d
	}

	first, err := config.EnsureProviderFile(dir)
	if err != nil {
		fmt.Fprintf(stderr, "caosi: 无法创建配置: %v\n", err)
		return 1
	}
	if first != nil {
		writeFirstRun(stderr, first.WrotePath, dir, *configDir != "")
		return 1
	}

	file, err := config.Load(dir)
	if err != nil {
		fmt.Fprintf(stderr, "caosi: 配置无效\n%v\n\n打开 %s 改好再运行。\n", err, config.ProviderFilePath(dir))
		return 1
	}

	level := parseLevel(*logLevel)
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
	srv := server.New(file, log)

	addr := net.JoinHostPort(*listen, fmt.Sprintf("%d", *port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "caosi: 无法监听 %s: %v\n换一个 --port，或检查是否已经有 caosi 在跑。\n", addr, err)
		return 1
	}

	httpSrv := &http.Server{Handler: srv.Handler()}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shCtx)
	}()

	go srv.WatchConfig(ctx, dir)
	warnPlaceholders(stderr, file)
	printReady(stderr, ln.Addr().String(), file)
	err = httpSrv.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "caosi: 服务退出: %v\n", err)
		return 1
	}
	return 0
}

func writeHelp(w io.Writer) {
	fmt.Fprint(w, `caosi — 本机 LLM 协议转换器

把 Claude Code / Codex / Gemini CLI 指到:
  http://127.0.0.1:9999/{provider_name}

用法:
  caosi [选项]

选项:
  --config-dir  配置目录（默认: ~/.caosi）
  --port        端口（默认: 9999）
  --listen      只允许 127.0.0.1 或 ::1（默认: 127.0.0.1）
  --log-level   debug|info|warn|error（默认: info）
  -h, --help    显示帮助

第一次运行会在配置目录写下 providers.jsonc 样例，填好密钥后再启动。
说明: 打开仓库中的 docs/guide.html
`)
}

func writeFirstRun(w io.Writer, path, dir string, customDir bool) {
	rerun := "caosi"
	if customDir {
		rerun = "caosi --config-dir " + dir
	}
	fmt.Fprintf(w, `caosi: 还没有配置文件，已经写好一份样例。

  文件: %s

下一步:
  1. 打开这个文件，填入 api_key；Claude Code 用 OpenAI 兼容上游时请保留 model
  2. 再运行: %s
  3. 用浏览器打开仓库里的 docs/guide.html

现在不会开始监听，避免空配置让人以为已经可用。
`, path, rerun)
}

func printReady(w io.Writer, addr string, file *config.File) {
	names := config.Names(file)
	sort.Strings(names)
	var b strings.Builder
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		p := file.Providers[n]
		b.WriteString(n)
		b.WriteString(" (")
		b.WriteString(p.Protocol.String())
		b.WriteString(")")
	}
	host := addr
	if strings.HasPrefix(host, "[::]") {
		host = "127.0.0.1" + strings.TrimPrefix(host, "[::]")
	}
	fmt.Fprintf(w, `caosi 正在监听 http://%s
  GET  /health
  POST /{provider_name}/v1/messages            Claude
  POST /{provider_name}/v1/chat/completions    OpenAI Chat
  POST /{provider_name}/v1/responses           OpenAI Responses
  POST /{provider_name}/v1beta/models/…:generateContent  Gemini

providers: %s

把客户端 Base URL 设成 http://%s/{provider_name}
说明: 仓库中的 docs/guide.html
`, host, b.String(), host)
}

func warnPlaceholders(w io.Writer, file *config.File) {
	for _, name := range config.Names(file) {
		p := file.Providers[name]
		key := strings.ToLower(p.APIKey)
		if key == "" || strings.Contains(key, "your-key") || strings.Contains(key, "sk-xxx") {
			fmt.Fprintf(w, "注意: Provider %q 的 api_key 还像占位符。上游会 401，先打开 %s 填真实密钥。\n", name, file.Path)
		}
	}
}

func isLoopback(listen string) bool {
	h := strings.TrimSpace(listen)
	if h == "127.0.0.1" || h == "::1" || h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
