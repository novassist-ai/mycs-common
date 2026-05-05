package rest_test

import (
	"context"
	"net/http"

	"github.com/novassist/mycs-common/pkg/goutils/rest"

	test_mocks "github.com/novassist/mycs-common/pkg/goutils/test/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rest Client", func() {

	var (
		err error

		testServer *test_mocks.MockHttpServer
	)

	type responseBody struct {
		Resparg1 *string `json:"resparg1,omitempty"`
		Resparg2 *string `json:"resparg2,omitempty"`
	}
	type responseError struct {
		Message *string `json:"message,omitempty"`
	}

	requestBody := struct {
		Arg1 string `json:"arg1,omitempty"`
		Arg2 string `json:"arg2,omitempty"`
		Arg3 string `json:"arg3,omitempty"`
	}{
		Arg1: "value1",
		Arg2: "value2",
		Arg3: "value3",
	}

	BeforeEach(func() {

		// start test server
		testServer = test_mocks.NewMockHttpServer(9096)
		testServer.ExpectCommonHeader("Content-Type", "application/json; charset=utf-8")
		testServer.ExpectCommonHeader("Accept", "application/json; charset=utf-8")
		testServer.Start()
	})

	AfterEach(func() {		
		testServer.Stop()
	})

	It("sends a rest post request and receives a good response", func() {

		testServer.PushRequest().
			ExpectPath("/api/a").
			ExpectMethod("POST").
			ExpectHeader("Api-Key", "12345").
			ExpectJSONRequest(restRequest).
			RespondWith(restResponse)

		responseBody := responseBody{}
		responseError := responseError{}
		response := &rest.Response{
			Body: &responseBody,
			Error: &responseError,
		}		

		restApiClient := rest.NewRestApiClient(context.Background(), "http://localhost:9096/api")
		err = restApiClient.NewRequest(
			&rest.Request{
				Path: "/a",
				Headers: rest.NV{
					"Api-Key": "12345",
				},
				Body: &requestBody,
			},
		).DoPost(response)
		Expect(err).ToNot(HaveOccurred())

		Expect(response.StatusCode).To(Equal(200))
		Expect(*responseBody.Resparg1).To(Equal("respvalue1"))
		Expect(*responseBody.Resparg2).To(Equal("respvalue2"))
		Expect(responseError.Message).To(BeNil())
	})

	It("sends a rest get request and receives an error response", func() {

		testServer.PushRequest().
			ExpectPath("/api/b").
			ExpectMethod("GET").
			ExpectQueryArg("arg1", "value1").
			ExpectQueryArg("arg2", "value2").
			RespondWithError(restErrorResponse, 400)

		responseBody := responseBody{}
		responseError := responseError{}
		response := &rest.Response{
			Body: &responseBody,
			Error: &responseError,
		}		

		restApiClient := rest.NewRestApiClient(context.Background(), "http://localhost:9096/api")
		err = restApiClient.NewRequest(
			&rest.Request{
				Path: "/b",
				QueryArgs: rest.NV{
					"arg1": "value1",
					"arg2": "value2",
				},
			},
		).DoGet(response)
		Expect(err).To(HaveOccurred())

		Expect(response.StatusCode).To(Equal(400))
		Expect(responseBody.Resparg1).To(BeNil())
		Expect(responseBody.Resparg2).To(BeNil())
		Expect(*responseError.Message).To(Equal("test error"))
	})

	It("sends an authenticated rest post request and receives a good response", func() {

		mockAuthCrypt, err := test_mocks.NewMockAuthCrypt("some key", nil)
		Expect(err).ToNot(HaveOccurred())

		testServer.PushRequest().
			ExpectPath("/api/a").
			ExpectMethod("POST").
			ExpectHeader("Api-Key", "12345").
			WithCallbackTest(test_mocks.HandleAuthHeaders(mockAuthCrypt, restRequest, restResponse))

		responseBody := responseBody{}
		responseError := responseError{}
		response := &rest.Response{
			Body: &responseBody,
			Error: &responseError,
		}		

		restApiClient := rest.NewRestApiClient(context.Background(), "http://localhost:9096/api").WithAuthCrypt(mockAuthCrypt)
		err = restApiClient.NewRequest(
			&rest.Request{
				Path: "/a",
				Headers: rest.NV{
					"Api-Key": "12345",
				},
				Body: &requestBody,
			},
		).DoPost(response)
		Expect(err).ToNot(HaveOccurred())

		Expect(response.StatusCode).To(Equal(200))
		Expect(*responseBody.Resparg1).To(Equal("respvalue1"))
		Expect(*responseBody.Resparg2).To(Equal("respvalue2"))
		Expect(responseError.Message).To(BeNil())
	})

	It("sends an authenticated rest post request and receives a bad response auth header", func() {

		mockAuthCrypt, err := test_mocks.NewMockAuthCrypt("some key", nil)
		Expect(err).ToNot(HaveOccurred())
	
		testServer.PushRequest().
			ExpectPath("/api/a").
			ExpectMethod("POST").
			ExpectHeader("Api-Key", "12345").
			WithCallbackTest(func(w http.ResponseWriter, r *http.Request, body string) *string {
				encryptedAuthToken := r.Header["X-Auth-Token"]
				Expect(encryptedAuthToken).NotTo(BeNil())		
				w.Header()["X-Auth-Token-Response"] = []string{ "bad response auth token" }
				return nil
			})
	
		responseBody := responseBody{}
		responseError := responseError{}
		response := &rest.Response{
			Body: &responseBody,
			Error: &responseError,
		}		
	
		restApiClient := rest.NewRestApiClient(context.Background(), "http://localhost:9096/api").WithAuthCrypt(mockAuthCrypt)
		err = restApiClient.NewRequest(
			&rest.Request{
				Path: "/a",
				Headers: rest.NV{
					"Api-Key": "12345",
				},
				Body: &requestBody,
			},
		).DoPost(response)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("response auth token is not valid"))
	})
})

const restRequest = `{"arg1":"value1","arg2":"value2","arg3":"value3"}`
const restResponse = `{"resparg1":"respvalue1","resparg2":"respvalue2"}`
const restErrorResponse = `{"message":"test error"}`
