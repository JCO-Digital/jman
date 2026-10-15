package commands

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

// loginTimeout is how long `jman login` waits for the browser approval.
const loginTimeout = 5 * time.Minute

var flagNoBrowser bool

var loginCmd = &cobra.Command{
	Use:   "login [api-url]",
	Short: "Log in to jman-api through the browser",
	Long: `Logs the CLI in to jman-api. It opens the jman web UI in your browser,
where you approve the login, and stores the resulting token for this device
in the system keyring (or a private file if there is no keyring).

The API URL is saved in the config file. It is required the first time and
optional afterwards; passing a different URL switches to that API.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runLogin,
}

var logoutCmd = &cobra.Command{
	Use:           "logout",
	Short:         "Log out of jman-api on this device",
	Long:          `Revokes this device's token in jman-api and deletes it and the local copy of the API data.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runLogout,
}

func init() {
	loginCmd.Flags().BoolVar(&flagNoBrowser, "no-browser", false, "print the login URL instead of opening a browser")
	rootCmd.AddCommand(loginCmd, logoutCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	apiURL := config.Cfg.APIURL
	if len(args) > 0 {
		var err error
		if apiURL, err = client.NormalizeURL(args[0]); err != nil {
			return err
		}
	}
	if apiURL == "" {
		return errors.New("no jman-api configured; run `jman login <api-url>`, e.g. `jman login https://jman.example.com`")
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()

	anon := client.New(apiURL, "")
	healthCtx, healthCancel := context.WithTimeout(ctx, 10*time.Second)
	health, err := anon.Health(healthCtx)
	healthCancel()
	if err != nil {
		return fmt.Errorf("can't reach jman-api at %s: %w", apiURL, err)
	}
	if health.WebURL == "" {
		return fmt.Errorf("jman-api at %s doesn't know its web UI address; an admin needs to set webUrl in its config.toml", apiURL)
	}

	verifier, err := randomString(32)
	if err != nil {
		return err
	}
	state, err := randomString(16)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	device, _ := os.Hostname()
	if device == "" {
		device = runtime.GOOS
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start the local login listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	results := make(chan loginCallback, 1)
	srv := &http.Server{Handler: loginCallbackHandler(state, results), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
	}()

	q := url.Values{}
	q.Set("port", strconv.Itoa(port))
	q.Set("state", state)
	q.Set("challenge", challenge)
	q.Set("device", device)
	loginURL := strings.TrimSuffix(health.WebURL, "/") + "/cli-login?" + q.Encode()

	if flagNoBrowser || openBrowser(loginURL) != nil {
		fmt.Printf("Open this URL in a browser on this machine to approve the login:\n\n  %s\n\n", loginURL)
	} else {
		fmt.Printf("Opened your browser to approve the login. If it didn't open, visit:\n\n  %s\n\n", loginURL)
	}
	fmt.Println("Waiting for approval...")

	var result loginCallback
	select {
	case result = <-results:
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("timed out waiting for the login to be approved")
		}
		return errors.New("login cancelled")
	}
	if result.err != nil {
		return result.err
	}

	exchangeCtx, exchangeCancel := context.WithTimeout(ctx, 30*time.Second)
	tok, err := anon.ExchangeCode(exchangeCtx, result.code, verifier)
	exchangeCancel()
	if err != nil {
		return fmt.Errorf("failed to complete the login: %w", err)
	}

	if previous := config.Cfg.APIURL; previous != apiURL {
		if err := config.SaveAPIURL(apiURL); err != nil {
			return err
		}
		if previous != "" {
			// The old token belongs to the other API.
			_ = credentials.Delete(previous)
		}
	}
	where, err := credentials.Set(apiURL, tok.Token)
	if err != nil {
		return fmt.Errorf("logged in, but failed to store the token: %w", err)
	}

	name := tok.User.DisplayName
	if name == "" {
		name = tok.User.Username
	}
	verb.Printf(verb.Normal, "%s Logged in to %s as %s (%s), level %s.\n", verb.Green("✓"), apiURL, name, tok.User.Username, tok.User.Level)
	if tok.User.RestrictedFrom != "" {
		verb.Printf(verb.Normal, "Your %s access is limited to %s until you enable two-factor authentication in the web UI.\n", tok.User.RestrictedFrom, tok.User.Level)
	}
	verb.Printf(verb.Verbose, "Token for %q stored in %s.\n", tok.DeviceName, where)

	snapCtx, snapCancel := context.WithTimeout(ctx, 30*time.Second)
	defer snapCancel()
	m, err := mirror.Get(snapCtx, mirror.Options{Force: true})
	if err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to download site data: %v\n", err)
		return nil
	}
	verb.Printf(verb.Normal, "Downloaded data for %d sites on %d servers.\n", len(m.Sites()), len(m.Servers()))
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	apiURL := config.Cfg.APIURL
	if apiURL == "" {
		return errors.New("not logged in")
	}
	token, err := credentials.Get(apiURL)
	if err == nil {
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()
		if err := client.New(apiURL, token).RevokeCurrentToken(ctx); err != nil && !errors.Is(err, client.ErrUnauthorized) {
			verb.PrintErrorf(verb.Normal, "Warning: couldn't revoke the token in jman-api (%v). Revoke it under Settings → CLI Tokens in the web UI.\n", err)
		}
	} else if !errors.Is(err, credentials.ErrNotFound) {
		return err
	}

	if err := credentials.Delete(apiURL); err != nil {
		return err
	}
	if err := mirror.Clear(); err != nil {
		return err
	}
	verb.Printf(verb.Normal, "Logged out of %s.\n", apiURL)
	return nil
}

// loginCallback is the outcome of the browser approval.
type loginCallback struct {
	code string
	err  error
}

// loginCallbackHandler receives the web UI's redirect after the user has
// approved or denied the login, and sends the outcome to results once.
func loginCallbackHandler(state string, results chan<- loginCallback) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "This login link doesn't match the running `jman login`.", http.StatusBadRequest)
			return
		}

		var result loginCallback
		message := "jman is logged in. You can close this tab and return to your terminal."
		switch {
		case q.Get("error") == "access_denied":
			result.err = errors.New("the login was denied in the browser")
			message = "The login was denied. You can close this tab."
		case q.Get("code") == "":
			result.err = errors.New("the browser returned no login code")
			message = "Something went wrong: no login code was returned. Run `jman login` again."
		default:
			result.code = q.Get("code")
		}

		select {
		case results <- result:
		default:
			message = "This login has already been completed."
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><meta charset=\"utf-8\"><title>jman login</title><p style=\"font-family:sans-serif;margin:3em\">%s</p>", html.EscapeString(message))
	})
	return mux
}

// openBrowser opens url in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// randomString returns n random bytes, base64url-encoded without padding.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random data: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
