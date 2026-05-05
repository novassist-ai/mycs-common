package mocks

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/rest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type MockHttpServer struct {
	server     *http.Server
	serverExit sync.WaitGroup

	expectCommonHeaders []nv
	expectRequests      []*request
}

type request struct {
	callbackTest func(w http.ResponseWriter, r *http.Request, body string) *string

	expectPath        string
	expectMethod      string
	expectHeaders     []nv
	expectQueryArgs   []nv
	expectJSONRequest interface{}
	expectRequestBody *string
	responseBody      *string

	httpError *string
	httpCode  int
}

type nv struct {
	name, value string
}

func NewMockHttpServer(port int) *MockHttpServer {

	ms := MockHttpServer{}
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/", ms.mockResponseReflector)

	ms.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: serveMux,
	}

	return &ms
}

func NewMockHttpsServer(port int) (*MockHttpServer, string, error) {

	var (
		err error

		serverCert *tls.Certificate
		caRootPEM  []byte
	)

	ms := MockHttpServer{}
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/", ms.mockResponseReflector)

	if serverCert, caRootPEM, err = certsetup(); err != nil {
		return nil, "", err
	}
	serverTLSConf := &tls.Config{
		Certificates: []tls.Certificate{*serverCert},
	}
	ms.server = &http.Server{
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   serveMux,
		TLSConfig: serverTLSConf,
	}
	return &ms, string(caRootPEM), nil
}

func (ms *MockHttpServer) Start() {

	// Bind the port in this goroutine before returning so the first client request
	// cannot connect before our listener owns the port (avoids flaky HTML/non-JSON).
	ln, err := net.Listen("tcp", ms.server.Addr)
	if err != nil {
		log.Fatalf("MockServer.Start(): listen on %s: %v", ms.server.Addr, err)
	}
	go func() {
		defer ms.serverExit.Done() // let caller know we are done cleaning up
		ms.serverExit.Add(1)

		var errServe error
		if ms.server.TLSConfig != nil {
			tlsLn := tls.NewListener(ln, ms.server.TLSConfig)
			errServe = ms.server.Serve(tlsLn)
		} else {
			errServe = ms.server.Serve(ln)
		}
		if errServe != nil && errServe != http.ErrServerClosed {
			log.Fatalf("MockServer.Start(): %v", errServe)
		}
	}()
}

func (ms *MockHttpServer) Stop() {
	if err := ms.server.Shutdown(context.Background()); err != nil {
		log.Fatalf("MockServer.Stop(): %v", err)
	}
	ms.serverExit.Wait()
}

func (ms *MockHttpServer) ExpectCommonHeader(name, value string) {
	ms.expectCommonHeaders = append(ms.expectCommonHeaders, nv{name, value})
}

func (ms *MockHttpServer) PushRequest() *request {
	request := &request{}
	ms.expectRequests = append(ms.expectRequests, request)
	return request
}

func (ms *MockHttpServer) Done() bool {
	return len(ms.expectRequests) == 0
}

func (ms *MockHttpServer) mockResponseReflector(w http.ResponseWriter, r *http.Request) {

	var (
		err    error
		exists bool
		value  []string

		buffer       bytes.Buffer
		size         int64
		requestBody  string
		responseBody *string

		hasError bool
	)

	logger.DebugMessage("MockServer: request URI: %s", r.RequestURI)
	logger.DebugMessage("MockServer: request Headers: %s", r.Header)

	if size, err = buffer.ReadFrom(r.Body); err != nil {
		http.Error(w,
			fmt.Sprintf("Error reading request body: %s", err.Error()),
			http.StatusBadRequest,
		)
		return
	}
	requestBody = buffer.String()
	logger.DebugMessage("MockServer: request Body (%d): %s", size, requestBody)

	// expected request
	if len(ms.expectRequests) == 0 {
		http.Error(w,
			"Error expected request stack is empty",
			http.StatusBadRequest,
		)
		return
	}
	expectedRequest := ms.expectRequests[0]
	ms.expectRequests = ms.expectRequests[1:]

	// check path
	if len(expectedRequest.expectPath) > 0 && expectedRequest.expectPath != r.URL.Path {
		http.Error(w,
			fmt.Sprintf(
				"Expecting path '%s' but got %s",
				expectedRequest.expectPath, r.URL.Path,
			),
			http.StatusBadRequest,
		)
		return
	}

	// check method
	if len(expectedRequest.expectMethod) > 0 && expectedRequest.expectMethod != r.Method {
		http.Error(w,
			fmt.Sprintf(
				"Expecting method '%s' but got %s",
				expectedRequest.expectMethod, r.Method,
			),
			http.StatusBadRequest,
		)
		return
	}

	// check expected headers
	checkHeaders := func(expectedHeaders []nv) bool {

		for _, header := range expectedHeaders {
			if value, exists = r.Header[header.name]; !exists {
				http.Error(w,
					fmt.Sprintf("Error expected header is missing: %s", header.name),
					http.StatusBadRequest,
				)
				return true
			}
			if len(value) == 0 {
				http.Error(w,
					fmt.Sprintf("Error expected header value was empty: %s", header.name),
					http.StatusBadRequest,
				)
				return true
			}
			if value[0] != header.value {
				http.Error(w,
					fmt.Sprintf(
						"Error expected header '%s' value does not match: expected '%s', got '%s'",
						header.name, header.value, value[0],
					),
					http.StatusBadRequest,
				)
				return true
			}
		}
		return false
	}

	// common headers
	if hasError = checkHeaders(ms.expectCommonHeaders); hasError {
		return
	}
	// request headers
	if hasError = checkHeaders(expectedRequest.expectHeaders); hasError {
		return
	}

	// request args
	queryArgs := r.URL.Query()
	for _, arg := range expectedRequest.expectQueryArgs {
		if value, exists = queryArgs[arg.name]; !exists || len(value) == 0 {
			http.Error(w,
				fmt.Sprintf(
					"Error expected query arg '%s' is missing",
					arg.name,
				),
				http.StatusBadRequest,
			)
			return
		}
		if value[0] != arg.value {
			http.Error(w,
				fmt.Sprintf(
					"Error expected header '%s' value does not match: expected '%s', got '%s'",
					arg.name, arg.value, value[0],
				),
				http.StatusBadRequest,
			)
			return
		}
	}

	// check expected request body
	if expectedRequest.expectJSONRequest != nil {

		var actual interface{}
		if err := json.Unmarshal([]byte(requestBody), &actual); err != nil {
			http.Error(w,
				fmt.Sprintf(
					"Error parsing JSON request body '%s': %s",
					requestBody, err.Error(),
				),
				http.StatusBadRequest,
			)
			return
		}

		if !reflect.DeepEqual(expectedRequest.expectJSONRequest, actual) {

			http.Error(w,
				fmt.Sprintf(
					"Error request body: expected '%v', got '%v'",
					expectedRequest.expectJSONRequest, actual,
				),
				http.StatusBadRequest,
			)
			return
		}

	} else if expectedRequest.expectRequestBody != nil &&
		*expectedRequest.expectRequestBody != requestBody {

		http.Error(w,
			fmt.Sprintf(
				"Error request body: expected '%s', got '%s'",
				*expectedRequest.expectRequestBody, requestBody,
			),
			http.StatusBadRequest,
		)
		return
	}

	// callback test
	responseBody = expectedRequest.responseBody
	if expectedRequest.callbackTest != nil {
		if respBody := expectedRequest.callbackTest(w, r, requestBody); respBody != nil {
			responseBody = respBody
		}
	}
	// return response
	if responseBody != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Connection", "close")
		if _, err = w.Write([]byte(*responseBody)); err != nil {
			http.Error(w,
				fmt.Sprintf("Error unable to return response: %s", err.Error()),
				http.StatusBadRequest,
			)
			return
		}
	}
	// return error
	if expectedRequest.httpError != nil {
		w.Header().Set("Connection", "close")
		http.Error(w, *expectedRequest.httpError, expectedRequest.httpCode)
	}
}

func (r *request) ExpectPath(path string) *request {
	r.expectPath = path
	return r
}

func (r *request) ExpectMethod(method string) *request {
	r.expectMethod = method
	return r
}

func (r *request) ExpectHeader(name, value string) *request {
	r.expectHeaders = append(r.expectHeaders, nv{name, value})
	return r
}

func (r *request) ExpectQueryArg(name, value string) *request {
	r.expectQueryArgs = append(r.expectQueryArgs, nv{name, value})
	return r
}

func (r *request) ExpectJSONRequest(body string) *request {

	var expected interface{}
	if err := json.Unmarshal([]byte(body), &expected); err != nil {
		log.Fatalf("Error parsing JSON request '%s': %s", body, err.Error())
	}

	r.expectJSONRequest = expected
	return r
}

func (r *request) ExpectRequest(body string) *request {
	r.expectRequestBody = &body
	return r
}

func (r *request) WithCallbackTest(cb func(w http.ResponseWriter, r *http.Request, body string) *string) *request {
	r.callbackTest = cb
	return r
}

func (r *request) RespondWith(body string) *request {
	r.responseBody = &body
	return r
}

func (r *request) RespondWithError(httpError string, code int) *request {
	r.httpError = &httpError
	r.httpCode = code
	return r
}

func HandleAuthHeaders(mockAuthCrypt rest.AuthCrypt, request, response string, validators ...func(expected, actual interface{}) bool) func(w http.ResponseWriter, r *http.Request, body string) *string {

	var expectedRequest interface{}
	if len(request) > 0 {
		if err := json.Unmarshal([]byte(request), &expectedRequest); err != nil {
			log.Fatalf("Error parsing JSON expected request body '%s': %s", request, err.Error())
		}
	}

	return func(w http.ResponseWriter, r *http.Request, body string) *string {
		defer GinkgoRecover()

		encryptedAuthToken := r.Header["X-Auth-Token"]
		Expect(encryptedAuthToken).NotTo(BeNil())
		Expect(len(encryptedAuthToken)).To(BeNumerically(">", 0))

		authRespToken := rest.NewResponseAuthToken(mockAuthCrypt)
		err := authRespToken.SetEncryptedToken(encryptedAuthToken[0])
		Expect(err).NotTo(HaveOccurred())
		err = authRespToken.ValidateTransportData(r)
		Expect(err).NotTo(HaveOccurred())

		// retrieve decrypted request payload
		if len(body) > 0 {
			var actualRequest interface{}
			err := authRespToken.DecryptAndDecodePayload(strings.NewReader(body), &actualRequest)
			Expect(err).ToNot(HaveOccurred())
			logger.DebugMessage("MockServer: decrypted and decoded request Body: %# v", actualRequest)
			if len(validators) > 0 {
				testEquality := false
				for _, v := range validators {
					testEquality = testEquality || v(expectedRequest, actualRequest)
				}
				if testEquality {
					Expect(expectedRequest).NotTo(BeNil())
					logger.DebugMessage("MockServer: expected request Body: %# v", expectedRequest)
					Expect(reflect.DeepEqual(expectedRequest, actualRequest)).To(BeTrue())
				}
			} else {
				Expect(expectedRequest).NotTo(BeNil())
				logger.DebugMessage("MockServer: expected request Body: %# v", expectedRequest)
				Expect(reflect.DeepEqual(expectedRequest, actualRequest)).To(BeTrue())
			}
		} else {
			Expect(expectedRequest).To(BeNil())
		}

		// get encrypted response body
		responseBody := []byte{}
		if len(response) > 0 {
			bodyReader, err := authRespToken.EncryptPayload(strings.NewReader(response))
			Expect(err).ToNot(HaveOccurred())
			responseBody, err = io.ReadAll(bodyReader)
			Expect(err).ToNot(HaveOccurred())
		}

		encryptedRespAuthToken, err := authRespToken.GetEncryptedToken()
		Expect(err).NotTo(HaveOccurred())

		w.Header()["X-Auth-Token-Response"] = []string{encryptedRespAuthToken}

		if len(responseBody) > 0 {
			respBody := string(responseBody)
			return &respBody
		} else {
			return nil
		}
	}
}
