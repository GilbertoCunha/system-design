package tests

import (
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var apiClient *APIClient = NewAPIClient()

var _ = Describe("Create short Urls works properly", func() {
	Context("When creating from a valid url", func() {
		It("Works when creating a new URL", func() {
			By("Creating a new URL")
			response, _, err := apiClient.CreateShortUrl("https://duckstuff.com")
			Expect(err).To(BeNil())
			By("Checking a successful status response")
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
		})

		It("Works when creating the same URL twice", func() {
			By("Creating a new URL")
			resp, s1, err := apiClient.CreateShortUrl("https://duckstuff1.com")
			Expect(err).To(BeNil())
			By("Checking a successful status response")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			By("Creating another new URL")
			resp, s2, err := apiClient.CreateShortUrl("https://duckstuff1.com")
			Expect(err).To(BeNil())
			By("Checking a successful status response")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			By("Ensuring their shortUrl are exactly the same")
			Expect(s1).To(Equal(s2))
		})

		It("Different URLs lead to different shortUrls", func() {
			By("Creating a new URL")
			resp, s1, err := apiClient.CreateShortUrl("https://shoulddiffer1.com")
			Expect(err).To(BeNil())
			By("Checking a successful status response")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			By("Creating another new URL")
			resp, s2, err := apiClient.CreateShortUrl("https://shoulddiffer2.com")
			Expect(err).To(BeNil())
			By("Checking a successful status response")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			By("Ensuring their shortUrl are exactly the same")
			Expect(s1).To(Not(Equal(s2)))
		})
	})

	DescribeTable("Fails when URL is invalid",
		func(url string) {
			By("Querying the API")
			response, _, err := apiClient.CreateShortUrl(url)
			Expect(err).To(BeNil())
			By("Ensuring a bad request status code")
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		},
		Entry("Using HTTP scheme", "http://google.com"),
		Entry("Using no scheme", "google.com"),
		Entry("Improperly formatted", "https:/google"),
	)
})

var _ = Describe("Retrieving longUrls works properly", func() {
	DescribeTable("Fails when invalid shortUrl is provided",
		func(shortUrl string) {
			By("Calling the API to retrieve a long URL")
			resp, _, err := apiClient.GetLongUrl(shortUrl)
			Expect(err).To(BeNil())
			By("Ensuring a bad request status code")
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		},
		Entry("Too short", "123"),
		Entry("Too long", strings.Repeat("a", 33)),
		Entry("Contains characters outside HEX range", strings.Repeat("a", 31)+"h"),
	)
})
