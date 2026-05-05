package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
)

type Authenticator struct {
	ctx context.Context

	authContext AuthToken
	oauthConfig *oauth2.Config

	authCallbackHandler func(w http.ResponseWriter, r *http.Request)

	// opaque value used to validate
	// against CSRF attacks
	state string

	localServerExit *sync.WaitGroup
	localHttpServer *http.Server
	serverError     error
}

func NewAuthenticator(
	ctx context.Context,
	authContext AuthToken,
	oauthConfig *oauth2.Config,
	authCallbackHandler func(w http.ResponseWriter, r *http.Request),
) (*Authenticator, context.CancelFunc) {

	cancellableCtx, cancelFunc := context.WithCancel(ctx)

	return &Authenticator{
		ctx: cancellableCtx,

		authContext: authContext,
		oauthConfig: oauthConfig,

		authCallbackHandler: authCallbackHandler,

		localServerExit: &sync.WaitGroup{},
	}, cancelFunc
}

// Starts an http listener locally to listen for
// the oauth redirect with authcode once the
// user has been authenticated by the auth service.
func (authn *Authenticator) StartOAuthFlow(
	ports []int,
	reqHandlers ...func() (string, func(http.ResponseWriter, *http.Request)),
) (string, error) {

	var (
		err error

		port int
	)

	var checkPort = func(port int) error {
		// check if there is a listener on the callback port
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)), time.Second)
		defer func() {
			if conn != nil {
				conn.Close()
			}
		}()
		return err
	}

	for _, port = range ports {
		if err = checkPort(port); err != nil {
			break
		}
	}
	if err == nil {
		return "", fmt.Errorf("unable to create callback server. all provided ports are in use")
	}

	// construct callback URL for auth code exchange
	authn.oauthConfig.RedirectURL = fmt.Sprintf(
		"http://localhost:%d/callback",
		port,
	)

	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/callback", authn.OAuthHandler)
	for _, reqHandler := range reqHandlers {
		pattern, handler := reqHandler()
		serveMux.HandleFunc(pattern, handler)
	}

	authn.localHttpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: serveMux,
	}

	// mutex to wait on until server shuts down
	authn.localServerExit.Add(1)

	go func() {
		// signal server has shutdown
		defer func() {
			authn.oauthConfig.RedirectURL = ""
			authn.localHttpServer = nil
			authn.localServerExit.Done()
		}()

		// always returns error. ErrServerClosed on graceful close
		if err = authn.localHttpServer.ListenAndServe(); err != http.ErrServerClosed {
			authn.serverError = err

			logger.DebugMessage(
				"Error serving local HTTP OAuth callback server: %# v",
				err)
		}
	}()

	// generate authorize URL where user will sign
	// in and redirect back to the local server
	authn.state = utils.RandomString(10)
	authURL := authn.oauthConfig.AuthCodeURL(authn.state)

	return authURL, nil
}

// Wait until OAuth flow has completed. Returns
// false if oath flow completes with callback
// to local server
func (authn *Authenticator) WaitForOAuthFlowCompletion(timeout time.Duration) (bool, error) {
	c := make(chan struct{})
	go func() {
		defer close(c)
		authn.localServerExit.Wait()
	}()
	select {
	case <-c:
		// server exited with callback
		// from authentication service
		return false, authn.serverError
	case <-authn.ctx.Done():
		// context cancelled so cancel
		// oauth flow
		authn.shutdownLocalHttpServer()
		return false, authn.ctx.Err()
	case <-time.After(timeout):
		// timed out
		return true, nil
	}
}

// Handles the OAuth callback which exchanges the
// auth code in the request for a token and saves
// the token.
func (authn *Authenticator) OAuthHandler(w http.ResponseWriter, r *http.Request) {

	var (
		err error
	)

	logger.TraceMessage(
		"Received authorization callback: %s",
		r.RequestURI)

	defer func() {
		authn.state = ""
	}()

	if err = r.ParseForm(); err != nil {
		http.Error(w, "Unable to parse request parameters", http.StatusBadRequest)
		return
	}
	state := r.Form.Get("state")
	if state != authn.state {
		http.Error(w, "State invalid", http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	if code == "" {
		http.Error(w, "Code not found", http.StatusBadRequest)
		return
	}
	if err = authn.RetrieveToken(code); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if authn.authCallbackHandler != nil {
		authn.authCallbackHandler(w, r)
	}

	if authn.localHttpServer != nil {
		go func() {
			// add a delay so any callback content
			// so response page can be served
			time.Sleep(time.Millisecond * 500)
			authn.shutdownLocalHttpServer()
		}()
	}
}

func (authn *Authenticator) shutdownLocalHttpServer() {

	if err := authn.localHttpServer.Shutdown(context.Background()); err != nil {
		authn.serverError = err

		logger.DebugMessage(
			"Error shutting down local HTTP OAuth callback server: %# v",
			err)
	}
}

// Exchange given auth code for a token
func (authn *Authenticator) RetrieveToken(authCode string) error {

	var (
		err error

		token *oauth2.Token
	)

	if token, err = authn.oauthConfig.Exchange(context.Background(), authCode); err != nil {
		return err
	}
	authn.authContext.SetToken(token)
	return nil
}

// Checks if the current auth context has been
// authenticated. This will refresh the oauth
// token if the access token has expired and
// the refresh token has not expired
func (authn *Authenticator) IsAuthenticated() (bool, error) {

	var (
		err   error
		token *oauth2.Token
	)

	token = authn.authContext.GetToken()
	if token == nil {
		return false, fmt.Errorf("not authenticated")
	}
	token.Expiry = time.Now()
	if token, err = authn.oauthConfig.TokenSource(authn.ctx, token).Token(); err != nil {
		errorMsg := err.Error()
		logger.DebugMessage("Token source refresh error: %s", errorMsg)

		if strings.ContainsAny(errorMsg, "token expired") {
			return false, fmt.Errorf("not authenticated")
		}
		return false, err
	}
	authn.authContext.SetToken(token)
	return true, nil
}
