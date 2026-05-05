package rest

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	crypto_rand "crypto/rand"

	"github.com/gin-gonic/gin"
	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/minio/highwayhash"
	"github.com/sirupsen/logrus"
)

type AuthToken interface {
	// updates with an encrypted token
	SetEncryptedToken(encryptedToken string) error
	// returns the encrypted auth token
	GetEncryptedToken() (string, error)

	// signs the data for the given keys in the transport
	// data request headers and "body" for a request and
	// response headers and "body" for a response. data
	// would point to either an http.Request or http.Response
	// instance.
	SignTransportData(keys []string, data interface{}) error
	ValidateTransportData(data interface{}) error

	// signs and encrypts the given payload
	EncryptPayload(payload io.Reader) (io.Reader, error)
  // decrypts the payload and validates the decrypted
	// payload checksum
	DecryptPayload(body io.Reader) (io.ReadCloser, error)
	DecryptAndDecodePayload(body io.Reader, obj interface{}) error

	// saves the token in the gin context
	SetInContext(c *gin.Context)
}

type authTokenCommon struct {
	authCrypt AuthCrypt

	// hash of the encrypted payload
	// associated with this token
	hashKey []byte

	transportDataChecksum string
	payloadChecksum       string
}
type requestAuthToken struct {
	authTokenCommon
}
type responseAuthToken struct {
	authTokenCommon
}

// an encrypted payload that is hashed for
// verification on request and response
type encryptedPayload struct {
	Payload string `json:"payload,omitempty"`
}

// creates an authenticated token to send with a request
func NewRequestAuthToken(authCrypt AuthCrypt) (AuthToken, error) {

	var (
		err error
	)

	authToken := &requestAuthToken{
		authTokenCommon: authTokenCommon{
			authCrypt: authCrypt,
		},
	}

	authToken.hashKey = make([]byte, 32)
	if _, err = io.ReadFull(crypto_rand.Reader, authToken.hashKey); err != nil {
		return nil, err
	}
	return authToken, nil
}

// a request token should be updated with the encrypted token
// response in order to authenticate that the response is
// associated with this token
func (t *requestAuthToken) SetEncryptedToken(encryptedToken string) error {

	var (
		err error

		responseToken string
	)

	crypt, cryptLock := t.authCrypt.Crypt()
	cryptLock.Lock()
	defer cryptLock.Unlock()

	if responseToken, err = crypt.DecryptB64(encryptedToken); err != nil {
		return err
	}
	logger.TraceMessage(
		"requestAuthToken.SetEncryptedToken: Reading response token data '%s'",
		responseToken)

	tokenParts := strings.Split(responseToken, "|")
	// only three parts expected as a response to an auth token does
	// not require the hash, which is already present
	if len(tokenParts) != 3 || tokenParts[0] != t.authCrypt.AuthTokenKey() {
		return fmt.Errorf("invalid response token")
	}
	t.authTokenCommon.transportDataChecksum = tokenParts[1]
	t.authTokenCommon.payloadChecksum = tokenParts[2]
	return nil
}

func (t *requestAuthToken) GetEncryptedToken() (string, error) {

	var (
		token strings.Builder
	)

	if t.authCrypt.IsAuthenticated() {
		crypt, cryptLock := t.authCrypt.Crypt()
		cryptLock.Lock()
		defer cryptLock.Unlock()

		token.WriteString(t.authCrypt.AuthTokenKey())
		token.WriteByte('|')
		token.WriteString(hex.EncodeToString(t.authTokenCommon.hashKey))
		token.WriteByte('|')
		token.WriteString(t.authTokenCommon.transportDataChecksum)
		token.WriteByte('|')
		token.WriteString(t.authTokenCommon.payloadChecksum)

		t := token.String()
		logger.TraceMessage("requestAuthToken.GetEncryptedToken: Auth token data: %s", t)
		return crypt.EncryptB64(t)
	}
	return "", fmt.Errorf("not authenticated")
}

func (t *requestAuthToken) SignTransportData(keys []string, data interface{}) error {

	var (
		err error
		ok  bool

		request *http.Request
		body    []byte

		checksum  strings.Builder
		dataValue strings.Builder

		hash hash.Hash
	)

	logger.TraceMessage(
		"requestAuthToken.SignTransportData(): Signing keys: %# v",
		keys,
	)

	if request, ok = data.(*http.Request); !ok {
		return fmt.Errorf("data instance needs to be of type *http.Request for request auth tokens")
	}
	for _, key := range keys {
		checksum.WriteString(key)
		checksum.Write([]byte{'~'})

		switch key {
			case "url": {
				dataValue.WriteString(request.URL.RequestURI())
			}
			case "body": {
				if body, err = ioutil.ReadAll(request.Body); err != nil {
					return err
				}
				dataValue.WriteString(string(body))
				request.Body = ioutil.NopCloser(bytes.NewBuffer(body))
			}
			default:
				value := request.Header.Get(key)
				dataValue.WriteString(string(value))
		}
	}
	dataToSign := dataValue.String()
	logger.TraceMessage(
		"requestAuthToken.SignTransportData(): Signing key data: %s",
		dataToSign,
	)

	// calculate hash of gathered data values
	if hash, err = highwayhash.New64(t.hashKey); err != nil {
		return err
	}
	if _, err = io.Copy(hash, bytes.NewBuffer([]byte(dataToSign))); err != nil {
		return err
	}
	checksum.WriteString(hex.EncodeToString(hash.Sum(nil)))
	t.authTokenCommon.transportDataChecksum = checksum.String()

	logger.TraceMessage(
		"requestAuthToken.SignTransportData(): Request transport checksum: %s",
		t.authTokenCommon.transportDataChecksum,
	)
	return nil
}

func (t *requestAuthToken) ValidateTransportData(data interface{}) error {

	var (
		err error
		ok  bool

		response *http.Response
		body     []byte

		dataValue strings.Builder

		hash hash.Hash
	)

	logger.TraceMessage(
		"requestAuthToken.ValidateTransportData(): Validating transport data checksum: %s",
		t.authTokenCommon.transportDataChecksum,
	)

	if response, ok = data.(*http.Response); !ok {
		return fmt.Errorf("data instance needs to be of type *http.Response for validating response to a request auth tokens")
	}

	parts := strings.Split(t.authTokenCommon.transportDataChecksum, "~")
	for _, key := range parts[:len(parts) - 1] {
		if key == "body" {
			if body, err = ioutil.ReadAll(response.Body); err != nil {
				return err
			}
			dataValue.WriteString(string(body))
			response.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		} else {
			value := response.Header.Get(key)
			dataValue.WriteString(string(value))
		}
	}
	dataToSign := dataValue.String()
	logger.TraceMessage(
		"requestAuthToken.ValidateTransportData(): Validating with checksum of: %s",
		dataToSign,
	)

	// calculate hash of gathered data values
	if hash, err = highwayhash.New64(t.hashKey); err != nil {
		return err
	}
	if _, err = io.Copy(hash, bytes.NewBuffer([]byte(dataToSign))); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != parts[len(parts) - 1] {
		return fmt.Errorf("response auth token transport data checksum validation failed")
	}
	return nil
}

func (t *requestAuthToken) SetInContext(c *gin.Context) {
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}
	c.Keys["authToken"] = t
}

// creates and authenticated response token from a request token
func NewResponseAuthToken(authCrypt AuthCrypt) AuthToken {

	return &responseAuthToken{
		authTokenCommon: authTokenCommon{
			authCrypt: authCrypt,
		},
	}
}

func (t *responseAuthToken) SetEncryptedToken(encryptedToken string) error {

	var (
		err error

		requestToken string
	)

	crypt, cryptLock := t.authCrypt.Crypt()
	cryptLock.Lock()
	defer cryptLock.Unlock()

	if requestToken, err = crypt.DecryptB64(encryptedToken); err != nil {
		return err
	}
	logger.TraceMessage(
		"responseAuthToken.SetEncryptedToken: Creating response token for request token '%s'",
		requestToken)

	tokenParts := strings.Split(requestToken, "|")
	if len(tokenParts) != 4 || tokenParts[0] != t.authCrypt.AuthTokenKey() {
		return fmt.Errorf("invalid request token")
	}
	if t.authTokenCommon.hashKey, err = hex.DecodeString(tokenParts[1]); err != nil {
		return fmt.Errorf("invalid request token. error parsing hash key: %s", err.Error())
	}
	t.authTokenCommon.transportDataChecksum = tokenParts[2]
	t.authTokenCommon.payloadChecksum = tokenParts[3]
	return nil
}

func (t *responseAuthToken) GetEncryptedToken() (string, error) {

	var (
		token strings.Builder
	)

	if t.authCrypt.IsAuthenticated() {
		crypt, cryptLock := t.authCrypt.Crypt()
		cryptLock.Lock()
		defer cryptLock.Unlock()

		token.WriteString(t.authCrypt.AuthTokenKey())
		token.WriteByte('|')
		token.WriteString(t.authTokenCommon.transportDataChecksum)
		token.WriteByte('|')
		token.WriteString(t.authTokenCommon.payloadChecksum)
		return crypt.EncryptB64(token.String())
	}
	return "", fmt.Errorf("not authenticated")
}

func (t *responseAuthToken) SignTransportData(keys []string, data interface{}) error {

	var (
		err error
		ok  bool

		response *http.Response
		body     []byte

		checksum  strings.Builder
		dataValue strings.Builder

		hash hash.Hash
	)

	logger.TraceMessage(
		"responseAuthToken.SignTransportData(): Signing keys: %# v",
		keys,
	)

	if response, ok = data.(*http.Response); !ok {
		return fmt.Errorf("data instance needs to be of type *http.Response for response auth tokens")
	}
	for _, key := range keys {
		checksum.WriteString(key)
		checksum.Write([]byte{'~'})

		if key == "body" {
			if body, err = ioutil.ReadAll(response.Body); err != nil {
				return err
			}
			dataValue.WriteString(string(body))
			response.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		} else {
			value := response.Header.Get(key)
			dataValue.WriteString(string(value))
		}
	}
	logger.TraceMessage(
		"responseAuthToken.SignTransportData(): Signing key data: %s",
		dataValue,
	)

	// calculate hash of gathered data values
	if hash, err = highwayhash.New64(t.authTokenCommon.hashKey); err != nil {
		return err
	}
	if _, err = io.Copy(hash, bytes.NewBuffer([]byte(dataValue.String()))); err != nil {
		return err
	}
	checksum.WriteString(hex.EncodeToString(hash.Sum(nil)))
	t.authTokenCommon.transportDataChecksum = checksum.String()

	logger.TraceMessage(
		"responseAuthToken.SignTransportData(): Request transport checksum: %s",
		t.authTokenCommon.transportDataChecksum,
	)
	return nil
}

func (t *responseAuthToken) ValidateTransportData(data interface{}) error {

	var (
		err error
		ok  bool

		request *http.Request
		body    []byte

		dataValue strings.Builder

		hash hash.Hash
	)

	logger.TraceMessage(
		"responseAuthToken.ValidateTransportData(): Validating transport data checksum: %s",
		t.authTokenCommon.transportDataChecksum,
	)

	if request, ok = data.(*http.Request); !ok {
		return fmt.Errorf("data instance needs to be of type *http.Request for validating a request")
	}

	parts := strings.Split(t.authTokenCommon.transportDataChecksum, "~")
	for _, key := range parts[:len(parts) - 1] {
		switch key {
			case "url": {
				dataValue.WriteString(request.URL.RequestURI())
			}
			case "body": {
				if body, err = ioutil.ReadAll(request.Body); err != nil {
					return err
				}
				dataValue.WriteString(string(body))
				request.Body = ioutil.NopCloser(bytes.NewBuffer(body))
			}
			default:
				value := request.Header.Get(key)
				dataValue.WriteString(string(value))
		}
	}
	dataToSign := dataValue.String()
	logger.TraceMessage(
		"responseAuthToken.ValidateTransportData(): Validating with checksum of: %s",
		dataToSign,
	)

	// calculate hash of gathered data values
	if hash, err = highwayhash.New64(t.hashKey); err != nil {
		return err
	}
	if _, err = io.Copy(hash, bytes.NewBuffer([]byte(dataToSign))); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != parts[len(parts) - 1] {
		return fmt.Errorf("request auth token transport data checksum validation failed")
	}
	return nil
}

func (t *responseAuthToken) SetInContext(c *gin.Context) {
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}
	c.Keys["authToken"] = t
}

// encrypts a given payload with the auth tokens auth crypt
func (t *authTokenCommon) EncryptPayload(payload io.Reader) (io.Reader, error) {

	var (
		err error

		waitForBodyRead sync.WaitGroup
		body, encryptedBody []byte

		hash hash.Hash
	)

	// load payload
	waitForBodyRead.Add(1)
	readerHash, writerHash := io.Pipe()
	readerBody, writerPayload := io.Pipe()

	go func() {
		defer func() {
			writerHash.Close()
			writerPayload.Close()
		}()

		writer := io.MultiWriter(writerHash, writerPayload)
		if _, err := io.Copy(writer, payload); err != nil {
			logger.ErrorMessage(
				"AuthToken.EncryptPayload(): Failed to copy payload for hashing and encryption: %s",
				err.Error())
		}
	}()
	go func() {
		defer waitForBodyRead.Done()

		// read payload content concurrently with hashing of payload content
		if body, err = io.ReadAll(readerBody); err != nil {
			logger.ErrorMessage(
				"AuthToken.EncryptPayload(): Failed to read body to encrypt: %s",
				err.Error())
		}
	}()

	// create checksum of payload content
	if hash, err = highwayhash.New64(t.hashKey); err != nil {
		return nil, err
	}
	if _, err = io.Copy(hash, readerHash); err != nil {
		return nil, err
	}
	t.payloadChecksum = hex.EncodeToString(hash.Sum(nil))

	logger.TraceMessage(
		"AuthToken.EncryptPayload(): Encrypted payload checksum: %s",
		string(t.payloadChecksum),
	)

	// encrypt payload content
	crypt, cryptLock := t.authCrypt.Crypt()
	cryptLock.Lock()
	defer cryptLock.Unlock()

	waitForBodyRead.Wait()
	if encryptedBody, err = crypt.Encrypt(body); err != nil {
		return nil, err
	}

	encryptedPayload := &encryptedPayload{
		Payload: base64.StdEncoding.EncodeToString(encryptedBody),
	}
	logger.TraceMessage(
		"AuthToken.EncryptPayload(): Encrypted payload: %# v",
		encryptedPayload)

	payloadReader, payloadWriter := io.Pipe()
	go func() {
		defer payloadWriter.Close()
		if err := json.NewEncoder(payloadWriter).Encode(encryptedPayload); err != nil {
			logger.ErrorMessage(
				"AuthToken.EncryptPayload(): Failed to encode JSON with encrypted payload: %s",
				err.Error())
		}
	}()
	return payloadReader, nil
}

// decrypts a given payload with the auth tokens auth crypt
func (t *authTokenCommon) DecryptPayload(body io.Reader) (io.ReadCloser, error) {

	var (
		err error

		encryptedPayload encryptedPayload

		decodedBody,
		decryptedBody,
		payload []byte

		waitForPayloadRead sync.WaitGroup

		hash hash.Hash
	)

	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		if payload, err = io.ReadAll(body); err != nil {
			return nil, err
		}
		logger.TraceMessage(
			"AuthToken.DecryptPayload(): Encrypted payload: %# v",
			string(payload))

		body = io.NopCloser(bytes.NewReader(payload))
	}

	// unmarshal JSON containing encrypted payload
	if err = json.NewDecoder(body).Decode(&encryptedPayload); err != nil {
		return nil, err
	}

	// decrypt payload
	crypt, cryptLock := t.authCrypt.Crypt()
	cryptLock.Lock()
	defer cryptLock.Unlock()

	if decodedBody, err = base64.StdEncoding.DecodeString(encryptedPayload.Payload); err != nil {
		return nil, err
	}
	if decryptedBody, err = crypt.Decrypt(decodedBody); err != nil {
		return nil, err
	}

	// load body
	waitForPayloadRead.Add(1)
	readerHash, writerHash := io.Pipe()
	readerPayload, writerBody := io.Pipe()

	go func() {
		defer func() {
			writerHash.Close()
			writerBody.Close()
		}()

		writer := io.MultiWriter(writerHash, writerBody)
		if _, err := io.Copy(writer, bytes.NewReader(decryptedBody)); err != nil {
			logger.ErrorMessage(
				"AuthToken.DecryptPayload: Failed to copy payload for hashing and decryption: %s",
				err.Error())
		}
	}()
	go func() {
		defer waitForPayloadRead.Done()

		// read payload content concurrently with hashing of payload content
		if payload, err = io.ReadAll(readerPayload); err != nil {
			logger.ErrorMessage(
				"AuthToken.DecryptPayload: Failed to read decrypted payload: %s",
				err.Error())
		}
	}()

	// create checksum of payload content
	if hash, err = highwayhash.New64(t.hashKey); err != nil {
		return nil, err
	}
	if _, err = io.Copy(hash, readerHash); err != nil {
		return nil, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != t.payloadChecksum {
		return nil, fmt.Errorf("received payload corrupted")
	}

	logger.TraceMessage(
		"AuthToken.DecryptPayload(): Decrypted payload checksum validates: %s",
		string(t.payloadChecksum),
	)

	waitForPayloadRead.Wait()
	return io.NopCloser(bytes.NewReader(payload)), nil
}

// decrypt and parse payload
func (t *authTokenCommon) DecryptAndDecodePayload(body io.Reader, obj interface{}) error {

	var (
		err error

		payload io.Reader
	)

	if payload, err = t.DecryptPayload(body); err != nil {
		return err
	}
	if err = json.NewDecoder(payload).Decode(&obj); err != nil {
		return err
	}
	return nil
}

// retrieves token from gin context
func DecryptPayloadFromContext(c *gin.Context, requestBody interface{}) error {

	var (
		err error
		ok  bool

		t         interface{}
		authToken AuthToken
	)

	if c.Keys == nil {
		return fmt.Errorf("gin context Keys attribute is nil")
	}
	if t, ok = c.Keys["authToken"]; !ok {
		return fmt.Errorf("auth token not found in gin context")
	}
	if authToken, ok = t.(AuthToken); !ok {
		return fmt.Errorf("auth token from gin context is of incorrect type: %T", t)
	}
	if err = authToken.DecryptAndDecodePayload(c.Request.Body, &requestBody); err != nil {
		return err
	}
	return nil
}

// Gin renderer for encrypted payloads
type RenderEncryptedPayload struct {
	context *gin.Context
	payload interface{}
}

func NewEncryptedRender(c *gin.Context, p interface{}) RenderEncryptedPayload {
	return RenderEncryptedPayload{
		context: c,
		payload: p,
	}
}

func (r RenderEncryptedPayload) WriteContentType(w http.ResponseWriter) {
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
}

func (r RenderEncryptedPayload) Render(w http.ResponseWriter) (error) {

	var (
		err error
		ok  bool

		token     interface{}
		authToken AuthToken

		response               io.Reader
		encryptedRespAuthToken string
	)

	payloadReader, payloadWriter := io.Pipe()
	go func() {
		defer payloadWriter.Close()
		if err = json.NewEncoder(payloadWriter).Encode(r.payload); err != nil {
			logger.ErrorMessage(
				"RenderEncryptedPayload.Render: Failed to encode JSON response payload: %s",
				err.Error())
		}
	}()

	if token, ok = r.context.Keys["authToken"]; !ok {
		return fmt.Errorf("auth token not found in context")
	}
	if authToken, ok = token.(AuthToken); !ok {
		return fmt.Errorf("auth token from context is of incorrect type: %T", token)
	}
	if response, err = authToken.EncryptPayload(payloadReader); err != nil {
		return err
	}
	if encryptedRespAuthToken, err = authToken.GetEncryptedToken(); err != nil {
		return err
	}
	w.Header().Add("X-Auth-Token-Response", encryptedRespAuthToken)

	// write response body
	if _, err = io.Copy(w, response); err != nil {
		return err
	}
	return nil
}
