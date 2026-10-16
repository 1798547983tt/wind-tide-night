package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/joho/godotenv"
)

const wtUsage = "usage: new-api wt emergency-login [--base-url https://your.site]"

// wind-tide: `new-api wt emergency-login` prints a single-use owner sign-in
// link (wind-tide/API.md §4). It only opens the database, so it can run next
// to the server; the link goes to stdout and nowhere else. The site address
// comes from --base-url or WT_SITE_URL, never from the database, where an
// administrator account could point it at another host; without either the
// command prints the path alone.
func runWtCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "emergency-login" {
		fmt.Fprintln(stderr, wtUsage)
		return 2
	}
	flags := flag.NewFlagSet("emergency-login", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprintln(stderr, wtUsage) }
	baseURL := flags.String("base-url", "", "the site's public address")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	_ = godotenv.Load(".env")
	common.InitEnv()
	// The command never connects to Redis, so the user cache that the audit
	// log reads must come from the database.
	common.RedisEnabled = false
	site := strings.TrimSpace(*baseURL)
	if site == "" {
		site = strings.TrimSpace(os.Getenv("WT_SITE_URL"))
	}
	if site != "" {
		parsed, err := url.Parse(site)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			fmt.Fprintln(stderr, "the site address must be an http or https URL such as https://your.site")
			return 2
		}
		site = strings.TrimRight(site, "/")
	}
	if err := model.WtCheckOwnerDiscordID(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := model.WtOpenDatabase(); err != nil {
		fmt.Fprintf(stderr, "open database: %v\n", err)
		return 1
	}
	defer func() {
		_ = model.CloseDB()
	}()
	owner, err := model.WtOwnerAccount()
	if err != nil {
		fmt.Fprintf(stderr, "owner account: %v\n", err)
		return 1
	}
	now := time.Now()
	token, err := model.IssueWtEmergencyToken(now)
	if err != nil {
		fmt.Fprintf(stderr, "issue emergency login token: %v\n", err)
		return 1
	}
	// The audit trail records that a link exists and until when, never the
	// link itself.
	model.RecordOperationAuditLog(owner.Id, owner.Role, "Emergency login link issued", "", "wt.emergency_link_issued",
		map[string]any{"expires_at": now.Add(model.WtEmergencyTokenTTL).Unix()}, nil, nil)
	fmt.Fprintf(stdout, "%s/emergency-login#token=%s\n", site, token)
	if site == "" {
		fmt.Fprintln(stderr, "No site address was given (--base-url or WT_SITE_URL): open the path above on the site's own address.")
	}
	fmt.Fprintf(stderr, "The link works once and expires in %d minutes; issuing another link cancels it.\n", int(model.WtEmergencyTokenTTL/time.Minute))
	return 0
}
