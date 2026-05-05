package mycsnode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/crypto"
	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/rest"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
)

type ApiClient struct {
	ctx context.Context
	
	refName,            // auth reference name (validates refName with idKey - i.e. user's device id key or app id key)
	refID,              // auth reference id (cross reference for idKey - i.e. user ID or app ID)
	clientIDKey  string // auth id cred
	clientRSAKey *crypto.RSAKey

	Node          SpaceNode
	nodePublicKey *crypto.RSAPublicKey

	authPath string

	keyTimeoutAt  int64
	crypt         *crypto.Crypt

	// mutex for api intialization
	initMutex sync.Mutex

	// client for authentication requests
	restAuthClient  *rest.RestApiClient
	keyRefreshMutex sync.Mutex

	authExecTimer *utils.ExecTimer

	// x-auth-key header
	AuthIDKey string

	// authenticated rest client for 
	// api requests
	RestApiClient *rest.RestApiClient

	// atomic flag indicating the
	// authentication status of the
	// rest api client
	isAuthenticated bool
	authTimeout     time.Duration
}

type ErrorResponse struct {
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

var authTimeout   = (10 * time.Second)/time.Millisecond // timeout waiting for auth in 10 seconds
var authRetryTime = (2 * time.Second)/time.Millisecond  // if auth fails then retry in 2 seconds

func init() {
	
	var (
		err error

		timeout string
		t       int
	)

	// override auth timeout
	if timeout = os.Getenv("CBS_NODE_AUTH_TIMEOUT"); len(timeout) > 0 {
		if t, err = strconv.Atoi(timeout); err == nil {
			authTimeout = time.Duration(t)
		}
	}
	// override auth retry timeout
	if timeout = os.Getenv("CBS_NODE_AUTH_RETRY_TIMEOUT"); len(timeout) > 0 {
		if t, err = strconv.Atoi(timeout); err == nil {
			authRetryTime = time.Duration(t)
		}
	}
}

func NewApiClient(
	refName, refID, 
	clientIDKey,
	clientRSAKeyPEM string,
	node SpaceNode,
	authPath string,
) (*ApiClient, error) {

	var (
		err error
	)

	apiClient := &ApiClient{ 
		Node: node,

		refName: refName,
		refID:   refID,
		clientIDKey:   clientIDKey,

		authPath: authPath,

		authTimeout: authTimeout,
	}
	if apiClient.nodePublicKey, err = crypto.NewPublicKeyFromPEM(node.GetPublicKey()); err != nil {
		return nil, err
	}
	if apiClient.clientRSAKey, err = crypto.NewRSAKeyFromPEM(clientRSAKeyPEM, nil); err != nil {
		return nil, err
	}
	
	apiClient.ctx = context.Background()
	// client used for authentication
	if apiClient.restAuthClient, err = node.RestApiClient(apiClient.ctx); err != nil {
		return nil, err
	}
	// client used for api invocation requests
	if apiClient.RestApiClient, err = node.RestApiClient(apiClient.ctx); err != nil {
		return nil, err
	}
	apiClient.RestApiClient = apiClient.RestApiClient.WithAuthCrypt(apiClient)

	return apiClient, nil
}

func NewUninitializedApiClient(
	refName, refID,
	authPath string,
) *ApiClient {

	apiClient := &ApiClient{ 		
		refName: refName,
		refID:   refID,

		authPath: authPath,

		authTimeout: authTimeout,
	}

	apiClient.ctx = context.Background()

	return apiClient
}

func (a *ApiClient) Initialize(
	clientIDKey string,
	clientRSAKey *crypto.RSAKey,
	node SpaceNode,
) (error) {

	var (
		err error
	)

	a.initMutex.Lock()
	defer a.initMutex.Unlock()

	a.clientIDKey = clientIDKey
	a.clientRSAKey = clientRSAKey

	a.Node = node
	if a.nodePublicKey, err = crypto.NewPublicKeyFromPEM(node.GetPublicKey()); err != nil {
		return err
	}

	// client used for authentication
	if a.restAuthClient, err = node.RestApiClient(a.ctx); err != nil {
		return err
	}
	// client used for api invocation requests
	if a.RestApiClient, err = node.RestApiClient(a.ctx); err != nil {
		return err
	}
	a.RestApiClient = a.RestApiClient.WithAuthCrypt(a)
	
	return nil
}

func (a *ApiClient) GetNode() SpaceNode {
	a.initMutex.Lock()
	defer a.initMutex.Unlock()

	return a.Node
}

func (a *ApiClient) IsRunning() bool {
	a.initMutex.Lock()
	defer a.initMutex.Unlock()

	return a.Node.GetStatus() == "running"
}

func (a *ApiClient) Start() error {
	a.authExecTimer = utils.NewExecTimer(a.ctx, a.AuthCallback, false)
	return a.authExecTimer.Start(0)
}

func (a *ApiClient) Stop() {
	if a.authExecTimer != nil {
		if err := a.authExecTimer.Stop(); err != nil {
			logger.DebugMessage(
				"ApiClient.Stop(): Auth execution timer stopped with err: %s", 
				err.Error())	
		}
	}
}

func (a *ApiClient) AuthCallback() (time.Duration, error) {

	var (
		err error

		isAuthenticated bool
	)

	if isAuthenticated, err = a.Authenticate(); err != nil {
		logger.ErrorMessage(
			"ApiClient.authCallback(): Authentication failed with err: %s", 
			err.Error())
	}
	if !isAuthenticated {
		return authRetryTime, nil
	}

	// re-authenticate 50ms before key expires
	return time.Duration(a.keyTimeoutAt - time.Now().UnixMilli() - 50), nil
}

func (a *ApiClient) Authenticate() (bool, error) {
	
	var (
		err error

		ecdhKey             *crypto.ECDHKey
		ecdhKeyPublicKey    string
		authReqKeyEncrypted string

		authReqKeyJSON,
		authRespKeyJSON []byte

		authResponse  AuthResponse
		errorResponse ErrorResponse

		encryptionKey []byte
	)

	a.keyRefreshMutex.Lock()
	defer a.keyRefreshMutex.Unlock()

	a.isAuthenticated = false

	if ecdhKey, err = crypto.NewECDHKey(); err != nil {
		return false, err
	}
	if ecdhKeyPublicKey, err = ecdhKey.PublicKey(); err != nil {
		return false, err
	}
	authReqKey := &AuthReqKey{
		RefID: a.refID,
		ECDHKey: ecdhKeyPublicKey,
		Nonce: time.Now().UnixMilli(),
	}
	if authReqKeyJSON, err = json.Marshal(authReqKey); err != nil {
		return false, err
	}
	logger.DebugMessage(
		"ApiClient.Authenticate(): created auth request key with nonce '%d': %# v", 
		authReqKey.Nonce, authReqKey)

	if authReqKeyEncrypted, err = a.nodePublicKey.EncryptBase64(authReqKeyJSON); err != nil {
		return false, err
	}
	authRequest := &AuthRequest{
		AuthReqIDKey: a.clientIDKey,
		AuthReqKey: authReqKeyEncrypted,
	}

	request := &rest.Request{
		Path: a.authPath,
		Body: authRequest,
	}
	response := &rest.Response{
		Body: &authResponse,
		Error: &errorResponse,
	}
	if err = a.restAuthClient.NewRequest(request).DoPost(response); err != nil {
		logger.ErrorMessage(
			"ApiClient.Authenticate(): HTTP error: %s", 
			err.Error())

		if len(errorResponse.ErrorMessage) > 0 {
			logger.ErrorMessage(
				"ApiClient.Authenticate(): Error message body: Error Code: %d; Error Message: %s", 
				errorResponse.ErrorCode, errorResponse.ErrorMessage)
	
			// todo: return a custom error type 
			// with parsed error object
			return false, fmt.Errorf("%s", errorResponse.ErrorMessage)	
		} else {
			return false, err
		}
	}

	if authRespKeyJSON, err = a.clientRSAKey.DecryptBase64(authResponse.AuthRespKey); err != nil {
		return false, err
	}
	authRespKey := &AuthRespKey{}
	if err = json.Unmarshal(authRespKeyJSON, authRespKey); err != nil {
		return false, err
	}
	logger.DebugMessage(
		"ApiClient.Authenticate(): received auth response key with nonce '%d': %# v", 
		authReqKey.Nonce, authRespKey)

	if authRespKey.RefName != a.refName || 
		authRespKey.Nonce != authReqKey.Nonce {
		
		return false, fmt.Errorf("invalid auth response")
	}	

	if encryptionKey, err = ecdhKey.SharedSecret(authRespKey.NodeECDHKey); err != nil {
		return false, err
	}
	if a.crypt, err = crypto.NewCrypt(encryptionKey); err != nil {
		return false, err
	}
	a.keyTimeoutAt = authRespKey.TimeoutAt
	a.AuthIDKey = authResponse.AuthRespIDKey

	a.isAuthenticated = true
	return true, nil
}

func (a *ApiClient) Reset() {

	a.keyRefreshMutex.Lock()
	defer a.keyRefreshMutex.Unlock()
	
	a.isAuthenticated = false
}

//
// rest.AuthCrypt implementation
//

func (a *ApiClient) IsAuthenticated() bool {

	a.keyRefreshMutex.Lock()
	defer a.keyRefreshMutex.Unlock()
		
	return a.isAuthenticated &&
		(time.Now().UnixNano() / int64(time.Millisecond)) < a.keyTimeoutAt
}

func (a *ApiClient) WaitForAuth() bool {
	
	if !a.IsAuthenticated() {
		timer := time.NewTicker(10 * time.Millisecond)
		defer timer.Stop()
	
		// trap ctrl-c
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		// timeoout
		timeoutAt := time.Duration(time.Now().UnixNano()) + a.authTimeout
	
		for {
			select {
			case <-c:
				return false
			case <-timer.C:
				if a.IsAuthenticated() {
					return true
				}
			}
			if time.Duration(time.Now().UnixNano()) > timeoutAt {
				logger.TraceMessage("Timedout waiting for successful authentication with the MyCS Rest API.")
				return false
			}
		}
	}
	return true
}

func (a *ApiClient) AuthTokenKey() string {
	return a.refName
}

func (a *ApiClient) Crypt() (*crypto.Crypt, *sync.Mutex) {
	return a.crypt, &a.keyRefreshMutex
}
