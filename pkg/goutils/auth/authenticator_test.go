package auth_test

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"
	"golang.org/x/oauth2"

	"github.com/novassist/mycs-common/pkg/goutils/auth"
	"github.com/novassist/mycs-common/pkg/goutils/logger"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Authenticator", func() {

	var (
		err        error
		oathServer *http.Server
		authn      *auth.Authenticator
	)

	authSync := sync.WaitGroup{}
	authSync.Add(1)

	testAuthContext := TestAuthContext{}

	if _, err = initOAuthTestServer(); err != nil {
		log.Fatalf("initOAuthTestServer(): %v", err)
	}
	serverExit := &sync.WaitGroup{}

	BeforeEach(func() {
		serverExit.Add(1)
		oathServer = startOAuthTestServer(serverExit)

		authn, _ = auth.NewAuthenticator(
			context.Background(),
			&testAuthContext,
			&oauth2.Config{
				ClientID:     "12345",
				ClientSecret: "qwerty",
				Scopes:       []string{"all"},
				// This points to the test Authorization Server
				// if our Client ID and Client Secret are valid
				// it will attempt to authorize our user
				Endpoint: oauth2.Endpoint{
					AuthURL:  "http://localhost:9096/authorize",
					TokenURL: "http://localhost:9096/token",
				},
			},
			func(w http.ResponseWriter, r *http.Request) {
				_, err = w.Write([]byte("authorized"))
				Expect(err).ToNot(HaveOccurred())
			},
		)
	})

	AfterEach(func() {
		err = oathServer.Shutdown(context.Background())
		Expect(err).ToNot(HaveOccurred())
		serverExit.Wait()
	})

	Context("oauth flow", func() {

		It("detects when context is not authenticated", func() {
			isAuthenticated, err := authn.IsAuthenticated()
			Expect(err).To(HaveOccurred())
			Expect(isAuthenticated).To(BeFalse())
			Expect(err.Error()).To(Equal("not authenticated"))
		})

		It("starts an oauth flow to authenticate a user", func() {

			var (
				matched bool
				wait    bool
				counter int
			)

			defer authSync.Done()
			Expect(testAuthContext.GetToken()).To(BeNil())

			// 9096 should be skipped as local server is listening on that port
			authUrl, err := authn.StartOAuthFlow(
				[]int{9096, 9094},
				// test handler
				func() (string, func(http.ResponseWriter, *http.Request)) {
					return "/test",
						func(w http.ResponseWriter, r *http.Request) {
							_, err = w.Write([]byte("test"))
							Expect(err).ToNot(HaveOccurred())
						}
				},
			)
			Expect(err).ToNot(HaveOccurred())
			u, err := url.Parse(authUrl)
			Expect(err).ToNot(HaveOccurred())

			Expect(strings.HasPrefix(authUrl, "http://localhost:9096/")).To(BeTrue())

			q := u.Query()
			Expect(q.Get("client_id")).To(Equal("12345"))
			Expect(q.Get("response_type")).To(Equal("code"))
			Expect(q.Get("scope")).To(Equal("all"))

			matched, _ = regexp.MatchString("[a-zA-Z0-9]+", q.Get("state"))
			Expect(matched).To(BeTrue())

			redirectUri := q.Get("redirect_uri")
			Expect(redirectUri).To(Equal("http://localhost:9094/callback"))

			u, err = url.Parse(redirectUri)
			Expect(err).ToNot(HaveOccurred())

			// check if there is a listener on the callback port
			conn, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", u.Port()), time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(conn).ToNot(BeNil())
			defer conn.Close()

			// check if callback server can serve a test response
			resp, err := http.Get("http://localhost:9094/test")
			Expect(err).ToNot(HaveOccurred())
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.Status).To(Equal("200 OK"))
			Expect(string(body)).To(Equal("test"))

			// simulate browser login via given auth URL
			go func() {
				defer GinkgoRecover()
				time.Sleep(time.Millisecond * 100)

				resp, err := http.Get(authUrl)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.Status).To(Equal("200 OK"))
				Expect(string(body)).To(Equal("authorized"))
			}()

			// wait for callback
			wait, err = authn.WaitForOAuthFlowCompletion(time.Millisecond * 10)
			Expect(err).ToNot(HaveOccurred())
			for counter = 0; wait; counter++ {
				wait, err = authn.WaitForOAuthFlowCompletion(time.Millisecond * 10)
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(counter).Should(BeNumerically(">", 0))

			// check listener on the callback port no longer exists
			_, err = net.DialTimeout("tcp", net.JoinHostPort("localhost", u.Port()), time.Second)
			Expect(err.Error()).To(Equal("dial tcp [::1]:9094: connect: connection refused"))

			// validate token exists
			token := testAuthContext.GetToken()
			Expect(token).ToNot(BeNil())
			Expect(len(token.AccessToken)).Should(BeNumerically(">", 0))
			Expect(len(token.RefreshToken)).Should(BeNumerically(">", 0))
		})

		It("detects when context is authenticated", func() {
			// wait for previous test to complete
			// so we have a valid token in store
			authSync.Wait()
			Expect(testAuthContext.GetToken()).ToNot(BeNil())

			prevToken := *testAuthContext.GetToken()
			isAuthenticated, err := authn.IsAuthenticated()
			Expect(err).ToNot(HaveOccurred())

			// check token validity
			Expect(isAuthenticated).To(BeTrue())

			// token should have been refreshed
			newToken := testAuthContext.GetToken()
			Expect(prevToken.AccessToken).ToNot(Equal(newToken.AccessToken))
			Expect(prevToken.RefreshToken).ToNot(Equal(newToken.RefreshToken))
		})
	})
})

func initOAuthTestServer() (*server.Server, error) {

	var err error

	clientStore := store.NewClientStore()
	if err = clientStore.Set("12345", &models.Client{
		ID:     "12345",
		Secret: "qwerty",
		Domain: "http://localhost:9094",
	}); err != nil {
		return nil, err
	}

	manager := manage.NewDefaultManager()
	manager.MustTokenStorage(store.NewMemoryTokenStore())
	manager.MapClientStorage(clientStore)

	refreshTokenCfg := manage.DefaultRefreshTokenCfg
	refreshTokenCfg.AccessTokenExp = time.Second * 5
	refreshTokenCfg.RefreshTokenExp = time.Second * 30
	manager.SetRefreshTokenCfg(refreshTokenCfg)

	srv := server.NewServer(server.NewConfig(), manager)
	srv.SetInternalErrorHandler(
		func(err error) (re *errors.Response) {
			Expect(err).ToNot(HaveOccurred())
			return
		},
	)
	srv.SetResponseErrorHandler(
		func(re *errors.Response) {
			Expect(err).ToNot(HaveOccurred())
		},
	)
	srv.SetUserAuthorizationHandler(
		func(w http.ResponseWriter, r *http.Request) (string, error) {
			// for testing purposes simply hard code user login
			// so the underlying server initializes and returns
			// the oauth tokens for testing
			return "user1", nil
		},
	)
	http.HandleFunc("/authorize",
		func(w http.ResponseWriter, r *http.Request) {
			logger.TraceMessage("TestServer: Authorization request received: %s", r.RequestURI)

			err := srv.HandleAuthorizeRequest(w, r)
			if err != nil {
				logger.TraceMessage("TestServer: Authorization request failed: %# v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
		},
	)
	http.HandleFunc("/token",
		func(w http.ResponseWriter, r *http.Request) {
			logger.DebugMessage("TestServer: Token request received: %s", r.RequestURI)

			err := srv.HandleTokenRequest(w, r)
			if err != nil {
				logger.TraceMessage("TestServer: Token request failed: %# v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		},
	)
	return srv, nil
}

func startOAuthTestServer(serverExit *sync.WaitGroup) *http.Server {

	httpSrv := &http.Server{Addr: ":9096"}
	go func() {
		defer serverExit.Done() // let caller know we are done cleaning up

		// always returns error. ErrServerClosed on graceful close
		if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			// unexpected error. port in use?
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	return httpSrv
}

/** Test Auth Context **/

type TestAuthContext struct {
	token *oauth2.Token
}

func (ac *TestAuthContext) SetToken(token *oauth2.Token) {
	ac.token = token
}

func (ac *TestAuthContext) GetToken() *oauth2.Token {
	return ac.token
}
