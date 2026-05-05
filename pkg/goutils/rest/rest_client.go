package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/sirupsen/logrus"
)

type RestApiClient struct {	
	ctx context.Context

	url        string
	httpClient *http.Client

	authCrypt AuthCrypt
}

type Request struct {
	Path       string
	Headers    NV
	QueryArgs  NV
	RawQuery   string
	Body       interface{}

	client *RestApiClient
}

type Response struct {
	StatusCode int
	Headers    NV

	Body  interface{}
	Error interface{}

	RawErrorMessage string
}

type NV map[string]string

func NewRestApiClient(ctx context.Context, url string) *RestApiClient {

	if strings.HasPrefix(url, "http://unix/") {
		socketPath := url[11:]
		return &RestApiClient{
			ctx: ctx,
			url: "http://unix",
			httpClient: &http.Client{
				Timeout: time.Second * 10,
				Transport: &http.Transport{
					DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
						return net.Dial("unix", socketPath)
					},
				},
			},
		}	

	} else {
		return &RestApiClient{
			ctx: ctx,
			url: url,
			httpClient: &http.Client{
				Timeout: time.Second * 10,
			},
		}	
	}
}

func (c *RestApiClient) WithHttpClient(httpClient *http.Client) *RestApiClient {
	c.httpClient = httpClient
	return c
}

func (c *RestApiClient) WithAuthCrypt(authCrypt AuthCrypt) *RestApiClient {
	c.authCrypt = authCrypt
	return c
}

func (c *RestApiClient) NewRequest(request *Request) *Request {
	request.client = c
	return request
}

func (r *Request) DoGet(response *Response) error {
	if r.Body != nil { 
		return fmt.Errorf("a body was provided for the get request to path %s", r.Path)
	}
	return r.do("GET", response)
}

func (r *Request) DoPost(response *Response) error {
	return r.do("POST", response)
}

func (r *Request) DoPut(response *Response) error {
	return r.do("PUT", response)
}

func (r *Request) DoDelete(response *Response) error {
	return r.do("DELETE", response)
}

func (r *Request) do(method string, response *Response) (err error) {

	var (
		err0 error
		url  strings.Builder
		
		body   []byte
		reader io.Reader
		writer io.WriteCloser

		authToken AuthToken

		httpRequest  *http.Request
		httpResponse *http.Response
	)

	logger.TraceMessage(
		"RestApiClient.Request.do(%s): processing request to: url: %s, headers: %+v, query args: %+v, body: %# v", 
		method, r.client.url, r.Headers, r.QueryArgs, r.Body,
	)

	if r.client.authCrypt != nil {
		if authToken, err = NewRequestAuthToken(r.client.authCrypt); err != nil {
			return err
		}
	}
	// keys to sign for authenticated requests. any additional 
	// provided headers will also be signed. body is not signed 
	// as it will be signed separately before encryption.
	keysToSign := []string{"url"}
	
	// concatonate client url with request 
	// path to create the complete url
	url.WriteString(r.client.url)
	if strings.HasSuffix(r.client.url, "/") {
		if strings.HasPrefix(r.Path, "/") {
			url.WriteString(r.Path[1:])
		} else {
			url.WriteString(r.Path)
		}
	} else {
		if strings.HasPrefix(r.Path, "/") {
			url.WriteString(r.Path)
		} else {
			url.Write([]byte{ '/' })
			url.WriteString(r.Path)
		}
	}

	if r.Body != nil {
		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			if body, err = json.Marshal(&r.Body); err != nil {
				return err
			}
			reader = bytes.NewReader(body)
		} else {
			reader, writer = io.Pipe()
			go func() {
				defer writer.Close()
				err0 = json.NewEncoder(writer).Encode(&r.Body)
			}()	
		}
		if authToken != nil {
			if reader, err = authToken.EncryptPayload(reader); err != nil {
				return err
			}
		}
	} else {
		reader = nil
	}
	if httpRequest, err = http.NewRequestWithContext(
		r.client.ctx, method, url.String(), reader,
	); err != nil {
		return err
	}

	// add headers
	httpRequest.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpRequest.Header.Set("Accept", "application/json; charset=utf-8")
	for n, v := range r.Headers {
		httpRequest.Header.Set(n, v)
		keysToSign = append(keysToSign, n)
	}

	// if client has an authenticated crypt then
	// add an encrypted authentication header to
	// the request to be validated on the server
	// side
	if authToken != nil {
		var encryptedReqToken string
		if err = authToken.SignTransportData(keysToSign, httpRequest); err != nil {
			return err
		}
		if encryptedReqToken, err = authToken.GetEncryptedToken(); err != nil {
			return err
		}
		httpRequest.Header.Set("X-Auth-Token", encryptedReqToken)
	}

	// add query params
	if len(r.QueryArgs) > 0 {
		query := httpRequest.URL.Query()
		for n, v := range r.QueryArgs {
			query.Add(n, v)
		}
		httpRequest.URL.RawQuery = query.Encode()	
	} else if len(r.RawQuery) > 0 {
		httpRequest.URL.RawQuery = r.RawQuery
	}
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		logger.TraceMessage(
			"RestApiClient.Request.do(%s): sending request:\n  url=%s\n  headers=%# v\n  body=%s",
			method,
			httpRequest.URL.String(),
			httpRequest.Header,
			string(body),
		)
	}
	if httpResponse, err = r.client.httpClient.Do(httpRequest); err != nil {
		return err
	}
	if err0 != nil {
		return err0
	}
	defer httpResponse.Body.Close()

	response.StatusCode = httpResponse.StatusCode
	response.Headers = make(map[string]string)
	for n, v := range httpResponse.Header {
		if (len(v) > 0) {
			response.Headers[n] = v[0]
		} else {
			response.Headers[n] = ""
		}
	}

	decodeBody := func(r io.Reader, v interface{}, buffer bool) error {
		if buffer || logrus.IsLevelEnabled(logrus.TraceLevel) {
			// retrieve response body to output to trace log
			// before unmarshalling to the response body value
			if body, err = io.ReadAll(r); err != nil {
				return err
			}
			return json.NewDecoder(bytes.NewReader(body)).Decode(v)
		} else {
			return json.NewDecoder(r).Decode(v)
		}
	}	

	// handle error responses
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusBadRequest {		
		if err = decodeBody(httpResponse.Body, response.Error, true); err != nil {
			response.RawErrorMessage = string(body)
			logger.WarnMessage("RestApiClient.Request.do(%s): Message body parse failed. Response body: %s", method, body)
		}
		err = fmt.Errorf("api error: %d - %s", httpResponse.StatusCode, httpResponse.Status)
	}

	// validate expected response auth token 
	// if a request auth token was created
	if err == nil {
		respBody := httpResponse.Body

		if authToken != nil {
			if encryptedRespToken, exists := response.Headers["X-Auth-Token-Response"]; exists {

				if err := authToken.SetEncryptedToken(encryptedRespToken); err != nil {
					if err != nil {
						logger.ErrorMessage(
							"RestApiClient.Request.do(%s): Failed to validate response auth token: %s",
							method, err.Error(),
						)
					}
					response.Error = nil
					return fmt.Errorf("response auth token is not valid")	
				}
				if respBody, err = authToken.DecryptPayload(respBody); err != nil {
					return err
				}

			} else {
				response.Error = nil
				return fmt.Errorf("response auth token header missing")
			}
		}
		err = decodeBody(respBody, response.Body, false)
	}

	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		logger.TraceMessage(
			"RestApiClient.Request.do(%s): received response:\n  url=%s\n  status code=%d\n  status=%s\n  headers=%# v\n  body=%s",
			method,
			httpRequest.URL.String(),
			httpResponse.StatusCode,
			httpResponse.Status,
			httpResponse.Header,
			string(body),
		)
	}

	return err
}
