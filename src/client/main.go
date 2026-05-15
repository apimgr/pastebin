// pastebin-cli — command-line client for the pastebin service.
//
// Usage:
//
//	pastebin-cli [--server URL] [--json] <command> [args]
//
// Commands:
//
//	create [file]             Create paste from stdin or file
//	get <id>                  Fetch raw paste content
//	delete <id> <token>       Delete paste using delete token
//	list [--limit N]          List recent public pastes
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// Version, Commit, and BuildDate are injected at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

const defaultServer = "http://localhost:8080"

func main() {
	log.SetFlags(0)
	log.SetPrefix("pastebin-cli: ")

	server := flag.String("server", envOrDefault("PASTEBIN_SERVER", defaultServer), "server base URL")
	asJSON := flag.Bool("json", false, "machine-readable JSON output")
	colorFlag := flag.String("color", "auto", "color output: auto, yes, no")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	// Honour NO_COLOR env var (https://no-color.org/) and --color flag.
	switch *colorFlag {
	case "no":
		os.Setenv("NO_COLOR", "1")
	case "yes":
		os.Unsetenv("NO_COLOR")
	}

	if *showVersion {
		fmt.Printf("pastebin-cli %s (commit %s, built %s)\n", Version, Commit, BuildDate)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON}

	switch args[0] {
	case "create":
		c.cmdCreate(args[1:])
	case "get":
		c.cmdGet(args[1:])
	case "delete", "del", "rm":
		c.cmdDelete(args[1:])
	case "list", "ls":
		c.cmdList(args[1:])
	case "version":
		fmt.Printf("pastebin-cli %s (commit %s, built %s)\n", Version, Commit, BuildDate)
	default:
		log.Fatalf("unknown command %q (try: create, get, delete, list)", args[0])
	}
}

// ─── Client ───────────────────────────────────────────────────────────────────

type client struct {
	server string
	asJSON bool
}

func (c *client) cmdCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	lang := fs.String("lang", "text", "syntax language")
	expiry := fs.String("expiry", "never", "expiry: 1h 1d 1w 1m 3m 6m 1y 2y never or seconds")
	burn := fs.Int("burn", 0, "delete after N views (0 = disabled)")
	unlisted := fs.Bool("unlisted", false, "create as unlisted (not shown in recent)")
	title := fs.String("title", "", "paste title")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	var content []byte
	var err error

	if fs.NArg() > 0 {
		content, err = os.ReadFile(fs.Arg(0))
		if err != nil {
			log.Fatalf("read file: %v", err)
		}
		// Auto-detect language from extension if not set.
		if *lang == "text" {
			*lang = detectLang(fs.Arg(0))
		}
		if *title == "" {
			*title = fs.Arg(0)
		}
	} else {
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("read stdin: %v", err)
		}
	}

	vis := 0
	if *unlisted {
		vis = 1
	}

	body := map[string]interface{}{
		"content":    string(content),
		"title":      *title,
		"language":   *lang,
		"expires_in": *expiry,
		"burn_after": *burn,
		"visibility": vis,
	}

	resp, err := c.postJSON("/api/v1/pastes", body)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("decode response: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		errMsg, _ := result["error"].(string)
		log.Fatalf("server error %d: %s", resp.StatusCode, errMsg)
	}

	if c.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	link, _ := result["link"].(string)
	token, _ := result["delete_token"].(string)
	fmt.Printf("URL:          %s\n", link)
	if token != "" {
		fmt.Printf("Delete token: %s\n", token)
		fmt.Println("(save the delete token — it will not be shown again)")
	}
}

func (c *client) cmdGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if fs.NArg() < 1 {
		log.Fatal("usage: get <id>")
	}
	id := fs.Arg(0)

	resp, err := c.get("/raw/" + id)
	if err != nil {
		log.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Fatalf("paste %q not found or has expired", id)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("server returned %d", resp.StatusCode)
	}

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		log.Fatalf("read response: %v", err)
	}
}

func (c *client) cmdDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if fs.NArg() < 2 {
		log.Fatal("usage: delete <id> <token>")
	}
	id, token := fs.Arg(0), fs.Arg(1)

	req, err := http.NewRequest(
		http.MethodDelete,
		c.url("/api/v1/pastes/"+id+"?token="+url.QueryEscape(token)),
		nil,
	)
	if err != nil {
		log.Fatalf("build request: %v", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Fatalf("paste %q not found or invalid token", id)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("server returned %d", resp.StatusCode)
	}

	if c.asJSON {
		io.Copy(os.Stdout, resp.Body)
		return
	}
	fmt.Printf("paste %s deleted\n", id)
}

func (c *client) cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	limit := fs.Int("limit", 20, "number of pastes to list (max 100)")
	page := fs.Int("page", 1, "page number")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	resp, err := c.get(fmt.Sprintf("/api/v1/pastes?page=%d&limit=%d", *page, *limit))
	if err != nil {
		log.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Pastes []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Language  string    `json:"language"`
			Views     int       `json:"views"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"pastes"`
		Pagination struct {
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("decode: %v", err)
	}

	if c.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	if len(result.Pastes) == 0 {
		fmt.Println("no pastes found")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tLANG\tVIEWS\tCREATED")
	for _, p := range result.Pastes {
		title := p.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			p.ID, title, p.Language, p.Views,
			p.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	tw.Flush()
	fmt.Printf("\n(%d total, page %d of %d)\n",
		result.Pagination.Total, *page, result.Pagination.TotalPages)
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *client) url(path string) string {
	return c.server + path
}

func (c *client) get(path string) (*http.Response, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return httpClient.Get(c.url(path))
}

func (c *client) postJSON(path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodPost, c.url(path), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return httpClient.Do(req)
}

// ─── Language detection ───────────────────────────────────────────────────────

func detectLang(filename string) string {
	ext := strings.ToLower(filename)
	if i := strings.LastIndex(ext, "."); i != -1 {
		ext = ext[i+1:]
	}
	m := map[string]string{
		"go": "go", "py": "python", "js": "javascript", "ts": "typescript",
		"rs": "rust", "java": "java", "c": "c", "cpp": "cpp", "cc": "cpp",
		"cs": "csharp", "php": "php", "rb": "ruby", "sh": "bash",
		"bash": "bash", "zsh": "bash", "ps1": "powershell",
		"html": "html", "htm": "html", "css": "css", "json": "json",
		"yaml": "yaml", "yml": "yaml", "toml": "toml", "xml": "xml",
		"sql": "sql", "md": "markdown", "txt": "text",
	}
	if lang, ok := m[ext]; ok {
		return lang
	}
	return "text"
}

// ─── Misc ─────────────────────────────────────────────────────────────────────

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `pastebin-cli %s — command-line client for the pastebin service

USAGE
    pastebin-cli [--server URL] [--json] <command> [flags] [args]

COMMANDS
    create [file]        Create paste from stdin or file; prints URL and delete token
    get <id>             Fetch and print raw paste content
    delete <id> <token>  Delete paste using its delete token
    list [--limit N]     List recent public pastes

CREATE FLAGS
    --lang <lang>        Syntax language (default: text)
    --expiry <duration>  1h 1d 1w 1m 3m 6m 1y 2y never, or seconds (default: never)
    --burn <n>           Delete after N views; 0 = disabled (default: 0)
    --unlisted           Create as unlisted (not shown in recent pastes)
    --title <title>      Paste title (optional)

LIST FLAGS
    --limit <n>          Number of pastes per page (default: 20)
    --page <n>           Page number (default: 1)

GLOBAL FLAGS
    --server <url>       Server base URL (default: %s or $PASTEBIN_SERVER)
    --json               Output machine-readable JSON
    --color <when>       Color output: auto, yes, no (default: auto; honors NO_COLOR)
    --version            Print version

EXAMPLES
    echo "Hello World" | pastebin-cli create --lang text
    cat myfile.go | pastebin-cli create --lang go --expiry 1d
    pastebin-cli create --burn 1 --expiry 1h secret.txt
    pastebin-cli get abc12345
    pastebin-cli delete abc12345 <delete-token>
    pastebin-cli list --limit 10

`, Version, defaultServer)
}
